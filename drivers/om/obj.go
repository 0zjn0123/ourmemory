package omdriver

import (
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/om"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
)

type obj struct {
	model.Object
	direct string
}

func (o *obj) URL() string { return o.direct }

func newDir(id, path, name string, mod time.Time, mask model.ObjMask) *obj {
	return &obj{Object: model.Object{
		ID: id, Path: path, Name: name, IsFolder: true, Modified: mod, Ctime: mod, Mask: mask,
	}}
}

func fileObj(u *model.User, f *om.File, mountPath string) *obj {
	rel := f.Path
	if rel == "" {
		rel = "/"
	}
	mask := model.ObjMask(0)
	if !f.IsDir && !ownerCanWrite(u, f) {
		mask = model.Locked
	}
	if f.IsDir && f.Scope == "shared" && !ownerCanWrite(u, f) {
		mask = model.Locked
	}
	o := &obj{Object: model.Object{
		ID:       f.ID,
		Path:     mountPath + rel,
		Name:     f.Name,
		Size:     f.Size,
		Modified: f.UpdatedAt,
		Ctime:    f.CreatedAt,
		IsFolder: f.IsDir,
		Mask:     mask,
		HashInfo: utils.HashInfo{},
	}}
	if !f.IsDir {
		url, _ := om.DriverURL(f, 24*time.Hour)
		o.direct = url
	}
	return o
}
