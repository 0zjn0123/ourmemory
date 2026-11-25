// Command omnode is the storage-contributor agent for ourmemory.
//
// It registers a machine with the main server, keeps a heartbeat, downloads
// the opaque archive chunks assigned to it, and reports them as replicated.
// It never receives private keys and cannot decrypt private vaults.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type nodeState struct {
	Server         string `json:"server"`
	NodeID         int    `json:"node_id"`
	Token          string `json:"token"`
	Name           string `json:"name"`
	CapacityBytes  int64  `json:"capacity_bytes"`
	PrivateCapable bool   `json:"private_capable"`
}

type apiEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func main() {
	var (
		server   = flag.String("server", env("OM_SERVER", "http://127.0.0.1:5259"), "main server base URL")
		secret   = flag.String("secret", env("OM_NODE_SECRET", ""), "node registration secret")
		name     = flag.String("name", env("OM_NODE_NAME", hostname()), "node display name")
		capacity = flag.Int64("capacity", envInt64("OM_NODE_CAPACITY_GB", 10)*1<<30, "bytes offered to the network (or use -gb)")
		gb       = flag.Int64("gb", 0, "capacity in GiB; overrides -capacity")
		dataDir  = flag.String("data", env("OM_NODE_DATA", "./omnode-data"), "local replica directory")
		username = flag.String("username", env("OM_NODE_USER", ""), "ourmemory username to credit")
		private  = flag.Bool("private", envBool("OM_NODE_PRIVATE", true), "store encrypted private-vault chunks too")
		limit    = flag.Int("limit", 10, "assignments per round")
		reset    = flag.Bool("reset", false, "forget local registration and register again")
	)
	flag.Parse()
	if *gb > 0 {
		*capacity = *gb << 30
	}
	if *capacity <= 0 {
		log.Fatal("-gb or -capacity must be positive")
	}
	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		log.Fatal(err)
	}
	statePath := filepath.Join(*dataDir, "node.json")
	state := nodeState{}
	if !*reset {
		if b, err := os.ReadFile(statePath); err == nil {
			_ = json.Unmarshal(b, &state)
		}
	}
	state.Server = strings.TrimRight(*server, "/")
	state.Name = *name
	state.CapacityBytes = *capacity
	state.PrivateCapable = *private

	client := &http.Client{Timeout: 30 * time.Second}
	if state.Token == "" || state.NodeID == 0 {
		if *secret == "" {
			log.Fatal("first run needs -secret (the server's OM_NODE_SECRET)")
		}
		out, err := register(client, state.Server, *secret, state, *username)
		if err != nil {
			log.Fatalf("register: %v", err)
		}
		state.NodeID = out.NodeID
		state.Token = out.Token
		if err := saveState(statePath, state); err != nil {
			log.Fatal(err)
		}
		log.Printf("registered node %d (%s), token saved to %s", state.NodeID, state.Name, statePath)
	} else {
		log.Printf("using saved node %d (%s)", state.NodeID, state.Name)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	hb := time.NewTicker(60 * time.Second)
	assign := time.NewTicker(5 * time.Minute)
	defer hb.Stop()
	defer assign.Stop()

	runRound(client, state, *dataDir, *limit)
	for {
		select {
		case <-stop:
			log.Print("stopping")
			return
		case <-hb.C:
			if err := heartbeat(client, state, dirSize(*dataDir)); err != nil {
				log.Printf("heartbeat: %v", err)
			}
		case <-assign.C:
			runRound(client, state, *dataDir, *limit)
		}
	}
}

type registerReply struct {
	NodeID int    `json:"node_id"`
	Token  string `json:"token"`
}

func register(client *http.Client, server, secret string, state nodeState, username string) (*registerReply, error) {
	body := map[string]any{
		"name": state.Name, "capacity_bytes": state.CapacityBytes,
		"private_capable": state.PrivateCapable, "username": username,
	}
	var out registerReply
	err := callJSON(client, server+"/api/om/nodes/register", "", map[string]string{"X-Node-Secret": secret}, body, &out)
	return &out, err
}

func heartbeat(client *http.Client, state nodeState, used int64) error {
	return callJSON(client, state.Server+"/api/om/nodes/heartbeat", state.Token, nil, map[string]any{"used_bytes": used}, nil)
}

func runRound(client *http.Client, state nodeState, dataDir string, limit int) {
	var wrap struct {
		Items []struct {
			BlobID string `json:"blob_id"`
			Size   int64  `json:"size"`
			URL    string `json:"url"`
		} `json:"items"`
	}
	url := fmt.Sprintf("%s/api/om/nodes/assign?limit=%d", state.Server, limit)
	if err := callJSON(client, url, state.Token, nil, nil, &wrap); err != nil {
		log.Printf("assign: %v", err)
		return
	}
	for _, it := range wrap.Items {
		path := filepath.Join(dataDir, it.BlobID+".blob")
		if st, err := os.Stat(path); err == nil && st.Size() == it.Size {
			_ = report(client, state, it.BlobID, "done")
			continue
		}
		if err := download(client, state, it.URL, path); err != nil {
			log.Printf("download %s: %v", it.BlobID, err)
			_ = report(client, state, it.BlobID, "failed")
			continue
		}
		if err := report(client, state, it.BlobID, "done"); err != nil {
			log.Printf("report %s: %v", it.BlobID, err)
			continue
		}
		log.Printf("replicated %s (%d bytes)", it.BlobID, it.Size)
	}
	_ = heartbeat(client, state, dirSize(dataDir))
}

func download(client *http.Client, state nodeState, url, path string) error {
	req, err := http.NewRequest(http.MethodGet, state.Server+url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Node-Token", state.Token)
	dl := &http.Client{Timeout: 0}
	resp, err := dl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	tmp := path + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func report(client *http.Client, state nodeState, blobID, status string) error {
	return callJSON(client, state.Server+"/api/om/nodes/report", state.Token, nil, map[string]any{"blob_id": blobID, "status": status}, nil)
}

func callJSON(client *http.Client, url, token string, headers map[string]string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(http.MethodPost, url, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Node-Token", token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if resp.StatusCode != http.StatusOK || env.Code != 200 {
		return fmt.Errorf("%s (http %d)", env.Message, resp.StatusCode)
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return err
		}
	}
	return nil
}

func saveState(path string, state nodeState) error {
	b, _ := json.MarshalIndent(state, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func dirSize(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "storage-node"
	}
	return h
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt64(k string, def int64) int64 {
	var v int64
	if _, err := fmt.Sscanf(os.Getenv(k), "%d", &v); err == nil {
		return v
	}
	return def
}

func envBool(k string, def bool) bool {
	switch strings.ToLower(os.Getenv(k)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}
