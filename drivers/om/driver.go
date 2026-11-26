package omdriver

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/om"
)

type OM struct {
	model.Storage
	Addition
}

func (d *OM) Config() driver.Config { return config }

func (d *OM) GetAddition() driver.Additional { return &d.Addition }

func (d *OM) Init(ctx context.Context) error {
	if om.Store == nil {
		return errors.New("ourmemory storage is not initialized")
	}
	return nil
}

func (d *OM) Drop(ctx context.Context) error { return nil }

func (d *OM) GetRoot(ctx context.Context) (model.Obj, error) {
	mod := time.Now()
	if d.GetStorage() != nil && !d.GetStorage().Modified.IsZero() {
		mod = d.GetStorage().Modified
	}
	return newDir("om-root", "/", "ourmemory", mod, model.Locked), nil
}

func (d *OM) GetRootPath() string { return "/" }

func userFrom(ctx context.Context) *model.User {
	if ctx == nil {
		return nil
	}
	u, _ := ctx.Value(conf.UserKey).(*model.User)
	return u
}

func ownerCanWrite(u *model.User, f *om.File) bool {
	return u != nil && (u.IsAdmin() || f.OwnerID == u.ID)
}

func cleanPath(p string) string {
	p = path.Clean("/" + strings.TrimSpace(p))
	if p == "." {
		return "/"
	}
	return p
}

func (d *OM) List(ctx context.Context, dir model.Obj, _ model.ListArgs) ([]model.Obj, error) {
	u := userFrom(ctx)
	p := cleanPath(dir.GetPath())
	switch {
	case p == "/":
		return []model.Obj{
			newDir("om-shared", "/shared", "shared", d.rootMod(), 0),
			newDir("om-private", "/private", "private", d.rootMod(), 0),
		}, nil
	case p == "/shared" || strings.HasPrefix(p, "/shared/"):
		rel := strings.TrimPrefix(p, "/shared")
		if rel == "" {
			rel = "/"
		}
		files, err := om.DriverList(u, "shared", rel)
		if err != nil {
			return nil, err
		}
		out := make([]model.Obj, 0, len(files))
		for i := range files {
			out = append(out, fileObj(u, &files[i], "/shared"))
		}
		return out, nil
	case p == "/private" || strings.HasPrefix(p, "/private/"):
		if u == nil || u.IsGuest() {
			return []model.Obj{}, nil
		}
		rel := strings.TrimPrefix(p, "/private")
		if rel == "" {
			rel = "/"
		}
		files, err := om.DriverList(u, "private", rel)
		if err != nil {
			return nil, err
		}
		out := make([]model.Obj, 0, len(files))
		for i := range files {
			out = append(out, fileObj(u, &files[i], "/private"))
		}
		return out, nil
	}
	return nil, errors.New("路径不存在")
}

func (d *OM) rootMod() time.Time {
	if d.GetStorage() != nil && !d.GetStorage().Modified.IsZero() {
		return d.GetStorage().Modified
	}
	return time.Now()
}

func (d *OM) Link(ctx context.Context, file model.Obj, _ model.LinkArgs) (*model.Link, error) {
	u := userFrom(ctx)
	f, err := om.DriverFileByID(u, file.GetID())
	if err != nil {
		return nil, err
	}
	if f.IsDir {
		return nil, errors.New("文件夹没有下载链接")
	}
	direct, ttl := om.DriverURL(f, 24*time.Hour)
	link := &model.Link{URL: direct, ContentLength: f.Size}
	if ttl > 0 {
		exp := ttl
		link.Expiration = &exp
	}
	return link, nil
}

func (d *OM) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, _ driver.UpdateProgress) error {
	u := userFrom(ctx)
	if u == nil || u.IsGuest() {
		return errors.New("请先登录")
	}
	scope, rel := splitScope(dstDir.GetPath())
	if scope == "" {
		return errors.New("只能上传到 shared 或 private")
	}
	_, err := om.DriverSave(u, scope, rel, file.GetName(), file.GetSize(), file.ModTime(), file)
	return err
}

func (d *OM) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	u := userFrom(ctx)
	if u == nil || u.IsGuest() {
		return errors.New("请先登录")
	}
	scope, rel := splitScope(parentDir.GetPath())
	if scope == "" {
		return errors.New("只能在这里创建 shared 或 private")
	}
	_, err := om.DriverMkdir(u, scope, rel, dirName)
	return err
}

func (d *OM) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	u := userFrom(ctx)
	f, err := om.DriverFileByID(u, srcObj.GetID())
	if err != nil {
		return err
	}
	_, err = om.DriverRename(u, f, newName)
	return err
}

func (d *OM) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	u := userFrom(ctx)
	f, err := om.DriverFileByID(u, srcObj.GetID())
	if err != nil {
		return err
	}
	if dScope, _ := splitScope(dstDir.GetPath()); dScope != f.Scope {
		return errors.New("不能跨共享盘/私人盘移动")
	}
	_, err = om.DriverMove(u, f, dstDir.GetPath())
	return err
}

func (d *OM) Remove(ctx context.Context, obj model.Obj) error {
	u := userFrom(ctx)
	f, err := om.DriverFileByID(u, obj.GetID())
	if err != nil {
		return err
	}
	return om.DriverRemove(u, f)
}

func splitScope(p string) (string, string) {
	p = cleanPath(p)
	switch {
	case p == "/shared":
		return "shared", "/"
	case strings.HasPrefix(p, "/shared/"):
		return "shared", strings.TrimPrefix(p, "/shared")
	case p == "/private":
		return "private", "/"
	case strings.HasPrefix(p, "/private/"):
		return "private", strings.TrimPrefix(p, "/private")
	default:
		return "", p
	}
}
