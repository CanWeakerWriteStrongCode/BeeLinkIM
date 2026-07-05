package mysqlx

import "time"

// TChatRoom 聊天房间表
type TChatRoom struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UID1      int64     `gorm:"index:idx_uid_pair,unique;not null" json:"uid1"` // 较小的UID
	UID2      int64     `gorm:"index:idx_uid_pair,unique;not null" json:"uid2"` // 较大的UID
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TChatMessage 聊天记录表
type TChatMessage struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	RoomID    int64     `gorm:"index;not null" json:"room_id"`
	FromUID   int64     `gorm:"index;not null" json:"from_uid"`
	ToUID     int64     `gorm:"index;not null" json:"to_uid"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	Sequence  int64     `gorm:"index:idx_room_seq,unique;not null" json:"sequence"` // 房间内唯一序号
	CreatedAt time.Time `json:"created_at"`
}

// TableName 自定义表名
func (TChatRoom) TableName() string    { return "chat_rooms" }
func (TChatMessage) TableName() string { return "chat_messages" }
