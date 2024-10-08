package service

import (
	"github.com/adiubaidah/wasabi/model"

	"gorm.io/gorm"
)

type HistoryServiceImpl struct {
	DB *gorm.DB
}

func NewHistoryService(DB *gorm.DB) HistoryService {
	return &HistoryServiceImpl{DB: DB}
}

func (s *HistoryServiceImpl) InsertHistory(processId uint, sender, receiver, content, role string) error {
	history := model.History{
		ProcessID: processId,
		Sender:    sender,
		Receiver:  receiver,
		Content:   content,
		RoleAs:    role,
	}
	return s.DB.Create(&history).Error
}

func (s *HistoryServiceImpl) GetHistory(sender string, receiver string) (*[]model.History, error) {
	var histories []model.History
	err := s.DB.Where("(sender = ? AND receiver = ?) OR (sender = ? AND receiver = ?)", sender, receiver, receiver, sender).
		Order("created_at ASC").
		Find(&histories).Error
	return &histories, err
}
