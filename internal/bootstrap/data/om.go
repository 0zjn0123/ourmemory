package data

import (
	"context"
	"errors"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/db"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/om"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// initOMStorage exposes the ourmemory pack as the OpenList root mount.
// OpenList's native file browser then lists /shared and /private through the
// OM storage driver instead of a second, independent filesystem.
func initOMStorage() {
	if om.Store == nil {
		return
	}
	if _, err := db.GetStorageByMountPath("/"); err == nil {
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Warnf("cannot inspect root storage: %v", err)
		return
	}
	if _, err := op.CreateStorage(context.Background(), model.Storage{
		MountPath: "/",
		Order:     0,
		Driver:    "OM",
		Addition:  `{}`,
	}); err != nil {
		log.Warnf("cannot create OpenList root storage for ourmemory: %v", err)
	}
}

// omBranding replaces the OpenList defaults that would otherwise show up inside
// the file browser. Only untouched defaults are rewritten so an administrator
// who customizes these settings in /files/@manage keeps their values.
func initOMBranding() {
	if om.Store == nil {
		return
	}
	type brand struct{ key, from, to string }
	items := []brand{
		{conf.SiteTitle, "OpenList", "ourmemory · 文件"},
		{conf.Logo, "https://res.oplist.org/logo/logo.svg", "/om-assets/logo.svg"},
		{conf.Favicon, "https://res.oplist.org/logo/logo.svg", "/om-assets/logo.svg"},
		{conf.MainColor, "#1890ff", "#1677ff"},
		{conf.Announcement, "Welcome to the OpenList project!\nFor the latest updates, to contribute code, or to submit suggestions and issues, please visit our [project repository](https://github.com/OpenListTeam/OpenList).", "ourmemory · 所有人的记忆，所有人的空间\n\n共享盘与私人盘都在同一个存储里；这里显示的文件就是站内所有子应用使用的文件。"},
	}
	changed := false
	for _, it := range items {
		cur, err := op.GetSettingItemByKey(it.key)
		if err != nil || cur == nil || cur.Value != it.from {
			continue
		}
		cur.Value = it.to
		if err := op.SaveSettingItem(cur); err != nil {
			log.Warnf("cannot rewrite setting %s: %v", it.key, err)
			continue
		}
		changed = true
	}
	if changed {
		op.SettingCacheUpdate()
	}
}
