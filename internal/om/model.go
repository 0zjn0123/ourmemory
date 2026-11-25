package om

import "time"

// Profile holds ourmemory-specific account data. The OpenList user table keeps
// credentials; this table keeps email verification, tier and private quota.
type Profile struct {
	UserID            uint      `gorm:"primaryKey" json:"user_id"`
	Email             string    `gorm:"size:255;index" json:"email"`
	EmailVerified     bool      `json:"email_verified"`
	DisplayName       string    `gorm:"size:80" json:"display_name"`
	Bio               string    `gorm:"type:text" json:"bio"`
	AvatarFileID      string    `gorm:"size:64" json:"avatar_file_id"`
	Tier              int       `gorm:"default:0" json:"tier"`
	PrivateEnabled    bool      `json:"private_enabled"`
	PrivateQuotaBytes int64     `gorm:"default:1073741824" json:"private_quota_bytes"`
	PrivateUsedBytes  int64     `gorm:"default:0" json:"private_used_bytes"`
	ContributionQuota int64     `gorm:"default:0" json:"contribution_quota_bytes"`
	CompressionLevel  int       `gorm:"default:7" json:"compression_level"`
	Coins             int       `gorm:"default:5" json:"coins"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (Profile) TableName() string { return "om_profiles" }

type PendingRegistration struct {
	ID           string    `gorm:"primaryKey;size:64" json:"id"`
	Username     string    `gorm:"size:64;index" json:"username"`
	Email        string    `gorm:"size:255;index" json:"email"`
	PasswordHash string    `gorm:"size:128" json:"-"`
	Salt         string    `gorm:"size:32" json:"-"`
	CodeHash     string    `gorm:"size:64" json:"-"`
	ExpiresAt    time.Time `json:"expires_at"`
	Attempts     int       `gorm:"default:0" json:"attempts"`
	CreatedAt    time.Time `json:"created_at"`
}

func (PendingRegistration) TableName() string { return "om_pending_registrations" }

type PrivateKey struct {
	UserID     uint      `gorm:"primaryKey" json:"user_id"`
	WrappedKey string    `gorm:"type:text" json:"-"` // base64(nonce||ciphertext), AES-GCM by OM_MASTER_KEY
	CreatedAt  time.Time `json:"created_at"`
}

func (PrivateKey) TableName() string { return "om_user_keys" }

type Pack struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	Scope     string    `gorm:"size:16;index" json:"scope"`
	OwnerID   uint      `gorm:"index" json:"owner_id"`
	Path      string    `gorm:"type:text" json:"path"`
	Size      int64     `json:"size"`
	Waste     int64     `json:"waste"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Pack) TableName() string { return "om_packs" }

type Blob struct {
	ID           string    `gorm:"primaryKey;size:64" json:"id"`
	PackID       string    `gorm:"size:64;index" json:"pack_id"`
	Offset       int64     `json:"offset"`
	StoredSize   int64     `json:"stored_size"`
	PlainSize    int64     `json:"plain_size"`
	SHA256       string    `gorm:"size:64;index" json:"sha256"`
	DedupKey     string    `gorm:"size:128;uniqueIndex" json:"dedup_key"`
	Encrypted    bool      `json:"encrypted"`
	RefCount     int       `gorm:"default:1" json:"ref_count"`
	TargetCopies int       `gorm:"default:2" json:"target_copies"`
	CreatedAt    time.Time `json:"created_at"`
}

func (Blob) TableName() string { return "om_blobs" }

type File struct {
	ID         string    `gorm:"primaryKey;size:64" json:"id"`
	OwnerID    uint      `gorm:"index" json:"owner_id"`
	Scope      string    `gorm:"size:16;index" json:"scope"` // shared|private
	ParentID   string    `gorm:"size:64;index" json:"parent_id"`
	Name       string    `gorm:"size:255" json:"name"`
	Path       string    `gorm:"type:text;index" json:"path"`
	Size       int64     `json:"size"`
	Mime       string    `gorm:"size:160" json:"mime"`
	Kind       string    `gorm:"size:32;index" json:"kind"` // file|image|video|audio|html|folder
	BlobID     string    `gorm:"size:64;index" json:"blob_id"`
	IsDir      bool      `json:"is_dir"`
	Visibility string    `gorm:"size:16;default:public" json:"visibility"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (File) TableName() string { return "om_files" }

type Title struct {
	ID    uint   `gorm:"primaryKey" json:"id"`
	Name  string `gorm:"size:64;uniqueIndex" json:"name"`
	Color string `gorm:"size:16" json:"color"`
	Kind  string `gorm:"size:32" json:"kind"`
}

func (Title) TableName() string { return "om_titles" }

type UserTitle struct {
	UserID    uint      `gorm:"primaryKey" json:"user_id"`
	TitleID   uint      `gorm:"primaryKey" json:"title_id"`
	GrantedAt time.Time `json:"granted_at"`
}

func (UserTitle) TableName() string { return "om_user_titles" }

type Video struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	FileID      string    `gorm:"size:64;index;not null" json:"file_id"`
	CoverFileID string    `gorm:"size:64" json:"cover_file_id"`
	Title       string    `gorm:"size:255;not null" json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	Category    string    `gorm:"size:64;index" json:"category"`
	Tags        string    `gorm:"size:512" json:"tags"`
	UploaderID  uint      `gorm:"index" json:"uploader_id"`
	Duration    int       `json:"duration"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	Views       int64     `gorm:"default:0" json:"views"`
	LikeCount   int64     `gorm:"default:0" json:"like_count"`
	CoinCount   int64     `gorm:"default:0" json:"coin_count"`
	FavCount    int64     `gorm:"default:0" json:"fav_count"`
	CommentCnt  int64     `gorm:"default:0" json:"comment_count"`
	DanmakuCnt  int64     `gorm:"default:0" json:"danmaku_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Video) TableName() string { return "om_videos" }

type Image struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	FileID      string    `gorm:"size:64;index;not null" json:"file_id"`
	Title       string    `gorm:"size:255" json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	Tags        string    `gorm:"size:512" json:"tags"`
	UploaderID  uint      `gorm:"index" json:"uploader_id"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	Views       int64     `gorm:"default:0" json:"views"`
	LikeCount   int64     `gorm:"default:0" json:"like_count"`
	FavCount    int64     `gorm:"default:0" json:"fav_count"`
	CommentCnt  int64     `gorm:"default:0" json:"comment_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Image) TableName() string { return "om_images" }

type Post struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"index" json:"user_id"`
	Content      string    `gorm:"type:text" json:"content"`
	RepostOf     uint      `gorm:"index;default:0" json:"repost_of"`
	RootID       uint      `gorm:"index;default:0" json:"root_id"`
	LikeCount    int64     `gorm:"default:0" json:"like_count"`
	CommentCount int64     `gorm:"default:0" json:"comment_count"`
	RepostCount  int64     `gorm:"default:0" json:"repost_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (Post) TableName() string { return "om_posts" }

type PostMedia struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	PostID uint   `gorm:"index" json:"post_id"`
	FileID string `gorm:"size:64;index" json:"file_id"`
	Sort   int    `json:"sort"`
}

func (PostMedia) TableName() string { return "om_post_media" }

type Reaction struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"uniqueIndex:idx_om_reaction" json:"user_id"`
	TargetType string    `gorm:"size:16;uniqueIndex:idx_om_reaction" json:"target_type"`
	TargetID   uint      `gorm:"uniqueIndex:idx_om_reaction" json:"target_id"`
	Kind       string    `gorm:"size:16;uniqueIndex:idx_om_reaction" json:"kind"` // like|coin|favorite
	Value      int       `gorm:"default:1" json:"value"`
	CreatedAt  time.Time `json:"created_at"`
}

func (Reaction) TableName() string { return "om_reactions" }

type Comment struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	TargetType string    `gorm:"size:16;index" json:"target_type"`
	TargetID   uint      `gorm:"index" json:"target_id"`
	UserID     uint      `gorm:"index" json:"user_id"`
	ParentID   uint      `gorm:"index;default:0" json:"parent_id"`
	Content    string    `gorm:"type:text" json:"content"`
	LikeCount  int64     `gorm:"default:0" json:"like_count"`
	CreatedAt  time.Time `json:"created_at"`
}

func (Comment) TableName() string { return "om_comments" }

type Danmaku struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	VideoID   uint      `gorm:"index" json:"video_id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	TimeMs    int64     `gorm:"index" json:"time_ms"`
	Mode      int       `gorm:"default:0" json:"mode"`
	Color     string    `gorm:"size:16" json:"color"`
	Content   string    `gorm:"size:200" json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func (Danmaku) TableName() string { return "om_danmaku" }

type WatchProgress struct {
	UserID    uint      `gorm:"primaryKey" json:"user_id"`
	VideoID   uint      `gorm:"primaryKey" json:"video_id"`
	Position  int       `json:"position"`
	Duration  int       `json:"duration"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (WatchProgress) TableName() string { return "om_watch_progress" }

type Follow struct {
	FollowerID uint      `gorm:"primaryKey" json:"follower_id"`
	FolloweeID uint      `gorm:"primaryKey" json:"followee_id"`
	CreatedAt  time.Time `json:"created_at"`
}

func (Follow) TableName() string { return "om_follows" }

type Hall struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:80;not null" json:"name"`
	Description string    `gorm:"size:512" json:"description"`
	OwnerID     uint      `gorm:"index" json:"owner_id"`
	JoinPolicy  string    `gorm:"size:16;default:open" json:"join_policy"`   // open|approval|closed
	ReadPolicy  string    `gorm:"size:16;default:public" json:"read_policy"` // public|members
	ChatFileID  string    `gorm:"size:64" json:"-"`
	CreatedAt   time.Time `json:"created_at"`
}

func (Hall) TableName() string { return "om_halls" }

type HallMember struct {
	HallID   uint      `gorm:"primaryKey" json:"hall_id"`
	UserID   uint      `gorm:"primaryKey" json:"user_id"`
	Role     string    `gorm:"size:16;default:member" json:"role"`
	Status   string    `gorm:"size:16;default:active" json:"status"` // active|pending
	JoinedAt time.Time `json:"joined_at"`
}

func (HallMember) TableName() string { return "om_hall_members" }

type Group struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Name       string    `gorm:"size:80;not null" json:"name"`
	OwnerID    uint      `gorm:"index" json:"owner_id"`
	ChatFileID string    `gorm:"size:64" json:"-"`
	CreatedAt  time.Time `json:"created_at"`
}

func (Group) TableName() string { return "om_groups" }

type GroupMember struct {
	GroupID  uint      `gorm:"primaryKey" json:"group_id"`
	UserID   uint      `gorm:"primaryKey" json:"user_id"`
	Role     string    `gorm:"size:16;default:member" json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

func (GroupMember) TableName() string { return "om_group_members" }

type Message struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	RoomType  string    `gorm:"size:16;index" json:"room_type"` // hall|group
	RoomID    uint      `gorm:"index" json:"room_id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Content   string    `gorm:"type:text" json:"content"`
	Kind      string    `gorm:"size:16;default:text" json:"kind"`
	FileID    string    `gorm:"size:64" json:"file_id"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (Message) TableName() string { return "om_messages" }

type APIKey struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	UserID     uint       `gorm:"index" json:"user_id"`
	Name       string     `gorm:"size:80" json:"name"`
	Prefix     string     `gorm:"size:16;index" json:"prefix"`
	Hash       string     `gorm:"size:64;uniqueIndex" json:"-"`
	Scopes     string     `gorm:"size:255" json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (APIKey) TableName() string { return "om_api_keys" }

type Node struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Name           string    `gorm:"size:80" json:"name"`
	URL            string    `gorm:"size:255" json:"url"`
	TokenHash      string    `gorm:"size:64;index" json:"-"`
	CapacityBytes  int64     `json:"capacity_bytes"`
	UsedBytes      int64     `json:"used_bytes"`
	PrivateCapable bool      `json:"private_capable"`
	Status         string    `gorm:"size:16;default:online" json:"status"`
	OwnerID        uint      `gorm:"index" json:"owner_id"`
	LastSeenAt     time.Time `json:"last_seen_at"`
	CreatedAt      time.Time `json:"created_at"`
	Disabled       bool      `json:"disabled"`
}

func (Node) TableName() string { return "om_nodes" }

type Replica struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	BlobID    string    `gorm:"size:64;index" json:"blob_id"`
	NodeID    uint      `gorm:"index" json:"node_id"`
	Status    string    `gorm:"size:16;default:pending" json:"status"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func (Replica) TableName() string { return "om_replicas" }

type ContributionEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	NodeID    uint      `gorm:"index" json:"node_id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Delta     int64     `json:"delta"`
	Reason    string    `gorm:"size:64" json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

func (ContributionEvent) TableName() string { return "om_contribution_events" }

type RedeemCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Code      string     `gorm:"size:64;uniqueIndex" json:"code"`
	Kind      string     `gorm:"size:16" json:"kind"` // tier|reservation|quota
	Tier      int        `json:"tier"`
	Bytes     int64      `json:"bytes"`
	Days      int        `json:"days"`
	UsedBy    uint       `gorm:"index;default:0" json:"used_by"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

func (RedeemCode) TableName() string { return "om_redeem_codes" }

type Reservation struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Bytes     int64     `json:"bytes"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

func (Reservation) TableName() string { return "om_reservations" }

type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Action    string    `gorm:"size:64;index" json:"action"`
	Target    string    `gorm:"size:255" json:"target"`
	IP        string    `gorm:"size:64" json:"ip"`
	Detail    string    `gorm:"type:text" json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

func (AuditLog) TableName() string { return "om_audit_logs" }

type Setting struct {
	Key   string `gorm:"primaryKey;size:80" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

func (Setting) TableName() string { return "om_settings" }
