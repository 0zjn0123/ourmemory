package om

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Init() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	Cfg = cfg
	dbPath := filepath.Join(cfg.DataDir, "om.db")
	gdb, err := gorm.Open(sqlite.Open(dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("open om.db: %w", err)
	}
	DB = gdb
	if err := DB.AutoMigrate(
		&Profile{}, &PendingRegistration{}, &PrivateKey{}, &Pack{}, &Blob{}, &File{},
		&Title{}, &UserTitle{}, &Video{}, &Image{}, &Post{}, &PostMedia{},
		&Reaction{}, &Comment{}, &Danmaku{}, &WatchProgress{}, &Follow{},
		&Hall{}, &HallMember{}, &Group{}, &GroupMember{}, &Message{},
		&APIKey{}, &Node{}, &Replica{}, &ContributionEvent{}, &RedeemCode{},
		&Reservation{}, &AuditLog{}, &Setting{},
	); err != nil {
		return fmt.Errorf("migrate om.db: %w", err)
	}
	if err := seed(); err != nil {
		return err
	}
	st, err := OpenStore(cfg)
	if err != nil {
		return err
	}
	Store = st
	log.Printf("[om] data=%s shared_max=%dMB cache_max=%dMB", cfg.DataDir, cfg.SharedMaxBytes>>20, cfg.CacheMaxBytes>>20)
	return nil
}

func seed() error {
	titles := []Title{
		{Name: "普通用户", Color: "#8b949e", Kind: "base"},
		{Name: "Ⅰ级捐赠", Color: "#58a6ff", Kind: "donation"},
		{Name: "Ⅱ级捐赠", Color: "#a371f7", Kind: "donation"},
		{Name: "Ⅲ级捐赠", Color: "#f778ba", Kind: "donation"},
		{Name: "存储贡献者", Color: "#3fb950", Kind: "storage"},
		{Name: "开发者", Color: "#d29922", Kind: "developer"},
		{Name: "管理员", Color: "#f85149", Kind: "admin"},
	}
	for _, t := range titles {
		var count int64
		DB.Model(&Title{}).Where("name = ?", t.Name).Count(&count)
		if count == 0 {
			if err := DB.Create(&t).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func EnsureProfile(userID uint, username string) (*Profile, error) {
	var p Profile
	err := DB.Where("user_id = ?", userID).First(&p).Error
	if err == nil {
		if p.DisplayName == "" {
			p.DisplayName = username
			DB.Save(&p)
		}
		if u, err := op.GetUserById(userID); err == nil && u.IsAdmin() {
			GrantTitle(userID, "管理员")
		}
		return &p, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	p = Profile{
		UserID:            userID,
		DisplayName:       username,
		PrivateQuotaBytes: Cfg.DefaultQuota,
		CompressionLevel:  7,
		Coins:             5,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	if err := DB.Create(&p).Error; err != nil {
		return nil, err
	}
	GrantTitle(userID, "普通用户")
	if u, err := op.GetUserById(userID); err == nil && u.IsAdmin() {
		GrantTitle(userID, "管理员")
	}
	return &p, nil
}

func GrantTitle(userID uint, name string) {
	var t Title
	if err := DB.Where("name = ?", name).First(&t).Error; err != nil {
		return
	}
	_ = DB.FirstOrCreate(&UserTitle{}, UserTitle{UserID: userID, TitleID: t.ID}).Error
}

func Audit(userID uint, action, target, ip, detail string) {
	_ = DB.Create(&AuditLog{UserID: userID, Action: action, Target: target, IP: ip, Detail: detail, CreatedAt: time.Now()}).Error
}

func DataDir() string {
	if Cfg.DataDir != "" {
		return Cfg.DataDir
	}
	d, _ := os.Getwd()
	return filepath.Join(d, "data")
}
