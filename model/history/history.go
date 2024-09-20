package history

import "time"

type History struct {
	ID        int       `gorm:"primaryKey;colum:id;autoIncrement"`
	Sender    string    `gorm:"colum:sender;type:varchar(255);not null"`
	Receiver  string    `gorm:"colum:receiver;type:varchar(255);not null"`
	Content   string    `gorm:"colum:content;type:text;not null"`
	RoleAs    string    `gorm:"colum:role_as;type:varchar(255);not null"`
	CreatedAt time.Time `gorm:"colum:created_at;autoCreateTime"`
}
