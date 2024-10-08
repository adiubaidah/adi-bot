package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/adiubaidah/wasabi/app"
	"github.com/adiubaidah/wasabi/helper"
	"github.com/adiubaidah/wasabi/model"

	"github.com/google/generative-ai-go/genai"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"gorm.io/gorm"
)

// Struct for storing user service status
type UserWaStatus struct {
	WaClient        *whatsmeow.Client
	Container       *sqlstore.Container
	IsActive        bool
	IsAuthenticated bool
	StartTime       time.Time
}

type ProcessServiceImpl struct {
	UserStatusMap map[string]*UserWaStatus // Map to track status by phone number
	WebSocketHub  *app.WebSocketHub
	Client        *genai.Client
	mu            sync.Mutex

	DB *gorm.DB
}

// Create new WhatsApp and AI service
func NewProcessService(client *genai.Client, waWebSocketHub *app.WebSocketHub, db *gorm.DB) ProcessService {
	return &ProcessServiceImpl{
		Client:        client,
		UserStatusMap: make(map[string]*UserWaStatus),
		WebSocketHub:  waWebSocketHub,
		DB:            db,
	}
}

func (s *ProcessServiceImpl) ListProcess() *[]model.ProcessWithUserDTO {
	processes := []model.ProcessWithUserDTO{}
	err := s.DB.Preload("User", func(db *gorm.DB) *gorm.DB {
		return db.Table("users").Select("id", "username", "role")

	}).Find(&processes).Error

	helper.PanicIfError("Error saat mengambil AI model:", err)
	for i := range processes {
		process := &processes[i]
		if status, exists := s.UserStatusMap[process.Phone]; exists {
			process.IsActive = status.IsActive
		} else {
			process.IsActive = false
		}
	}

	return &processes
}

// Function to activate WhatsApp and AI for a user
func (s *ProcessServiceImpl) Activate(phone string) *UserWaStatus {
	// If the service is already active, no need to activate again
	s.mu.Lock()
	defer s.mu.Unlock()
	if status, exists := s.UserStatusMap[phone]; exists && status.IsActive {
		return status
	}

	waClient, container := app.GetWaClient(phone)
	// Initialize a new status if it doesn't exist
	s.UserStatusMap[phone] = &UserWaStatus{
		WaClient:        waClient,
		Container:       container,
		IsActive:        false,
		IsAuthenticated: false,
		StartTime:       time.Now(),
	}

	// Set the current status variable for convenience
	status := s.UserStatusMap[phone]
	status.IsAuthenticated = status.WaClient.Store.ID != nil
	status.IsActive = true
	fmt.Println("Wa Status", status.IsAuthenticated)

	// If not authenticated, handle the authentication process
	if !status.IsAuthenticated {
		s.handleQRCodeAuthentication(phone, status, s.DB)
	} else {
		status.WaClient.Connect()

		s.WebSocketHub.SendMessage(phone, &model.WebResponse{
			Code:   200,
			Status: "success",
			Data:   "Connection Successful!",
		})
	}
	err := s.DB.Model(&model.Process{}).Where("phone = ?", phone).Updates(&model.Process{
		IsAuthenticated: status.IsAuthenticated,
	}).Error

	helper.PanicIfError("Error updating AI model", err)

	return status
}

// handleQRCodeAuthentication manages the WhatsApp authentication process using QR code
func (s *ProcessServiceImpl) handleQRCodeAuthentication(phone string, status *UserWaStatus, db *gorm.DB) {
	context := context.Background()
	qrChan, err := status.WaClient.GetQRChannel(context)
	helper.PanicIfError("Error getting QR channel", err)
	err = status.WaClient.Connect()
	helper.PanicIfError("Error connecting to WhatsApp", err)
	go func() {
		for evt := range qrChan {
			fmt.Println("QR event received:", evt.Event)
			switch evt.Event {
			case "code":
				qrPath := fmt.Sprintf("public/qr-%s-%s.png", time.Now().Format("20060102-150405"), phone)
				err := qrcode.WriteFile(evt.Code, qrcode.Medium, 256, qrPath)
				if err != nil {
					fmt.Println("Error generating QR code:", err)
				} else {
					fmt.Printf("QR code generated for phone %s. Scan it using WhatsApp!\n", phone)
					s.WebSocketHub.SendMessage(phone, &model.WebResponse{
						Code:   200,
						Status: "success",
						Data: map[string]string{
							"type":   "authenticating",
							"qrPath": qrPath,
						},
					})
				}
			case "success":
				s.mu.Lock()
				defer s.mu.Unlock()
				status.IsAuthenticated = true
				s.WebSocketHub.SendMessage(phone, &model.WebResponse{
					Code:   200,
					Status: "success",
					Data: map[string]string{
						"type": "authenticated",
					},
				})
				db.Model(&model.Process{}).Where("phone = ?", phone).Update("is_authenticated", true)
				return
			case "timeout":
				fmt.Println("Timeout")
				status.WaClient.Disconnect()
				s.WebSocketHub.SendMessage(phone, &model.WebResponse{
					Code:   408,
					Status: "error",
					Data: map[string]string{
						"type": "timeout",
					},
				})
				if stopCh, exists := app.ActiveRoutines[phone]; exists {
					s.Deactivate(phone)
					close(stopCh)                     // Close the stop channel to signal the Goroutine to stop
					delete(app.ActiveRoutines, phone) // Remove the phone from the map
				}
				return
			default:
				fmt.Println("Unhandled QR event:", evt.Event)
			}
		}

	}()
}

// CheckActivation checks if WhatsApp service is active for the given phone number
func (s *ProcessServiceImpl) Deactivate(phone string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if status exists and deactivate the service
	if status, exists := s.UserStatusMap[phone]; exists {
		status.IsActive = false
		status.WaClient.Disconnect()
		err := status.Container.Close()
		helper.PanicIfError("Error closing SQL container", err)
		return true
	}
	return false
}

func (s *ProcessServiceImpl) CheckActivation(phone string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if status exists and return its IsActive status
	if status, exists := s.UserStatusMap[phone]; exists {
		return status.IsActive
	}
	return false
}

func (s *ProcessServiceImpl) CheckAuthentication(phone string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	modelProcess := &model.Process{}
	// Check if status exists and return its IsAuthenticated status
	err := s.DB.Take(modelProcess, "phone = ?", phone).Error
	helper.PanicIfError("Error saat mengambil AI model:", err)

	return modelProcess.IsAuthenticated
}

func (service *ProcessServiceImpl) GetModel(userId uint) *model.Process {
	// Extract user information from context

	// Find AI model by user ID
	aiModel := &model.Process{}
	result := service.DB.Where("user_id = ?", userId).Take(&aiModel)

	// Check if record was not found
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		// Panic on unexpected errors
		helper.PanicIfError("Unexpected error", result.Error)
	}

	return aiModel

}

func (service *ProcessServiceImpl) UpsertModel(userId uint, modelAi model.CreateProcessModel) *model.Process {

	// Find AI model by user ID
	aiModel := &model.Process{}
	log.Default().Println("Test")
	result := service.DB.Where("user_id = ?", userId).Take(&aiModel)

	// Check if record was not found
	var err error
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		// Panic on unexpected errors
		panic(result.Error)
	}

	if result.RowsAffected == 0 {
		// If no AI model exists for this user, create a new one
		newAiModel := &model.Process{
			UserID: userId,
			CreateProcessModel: model.CreateProcessModel{
				Name:        modelAi.Name,
				Phone:       modelAi.Phone,
				Instruction: modelAi.Instruction,
				Temperature: modelAi.Temperature,
				TopK:        modelAi.TopK,
				TopP:        modelAi.TopP,
			},
			IsAuthenticated: false,
		}

		err = service.DB.Create(&newAiModel).Error
		if err != nil {
			panic(err)
		}
		aiModel = newAiModel // Assign new AI model to return later
	} else {
		// Update existing AI model
		fmt.Println("Update existing AI model")
		err = service.DB.Model(&aiModel).Where("user_id = ?", userId).Updates(&model.CreateProcessModel{
			Name:        modelAi.Name,
			Phone:       modelAi.Phone,
			Instruction: modelAi.Instruction,
			Temperature: modelAi.Temperature,
			TopK:        modelAi.TopK,
			TopP:        modelAi.TopP,
		}).Error
		helper.PanicIfError("Error updating AI model", err)
	}

	// Return the updated/new AI model
	return aiModel
}

func (service *ProcessServiceImpl) GenerateResponse(modelAi *model.Process, histories *[]model.History, input string) string {
	context := context.Background()
	var sessionHistory []*genai.Content
	for _, history := range *histories {
		sessionHistory = append(sessionHistory, &genai.Content{
			Role:  history.RoleAs,
			Parts: []genai.Part{genai.Text(history.Content)},
		})
	}

	// Generate response
	option := app.AiModelOption{
		Instruction: modelAi.Instruction,
		TopK:        modelAi.TopK,
		TopP:        modelAi.TopP,
		Temperature: modelAi.Temperature,
	}

	model := app.GetAIModel(service.Client, &option)
	session := model.StartChat()

	if (sessionHistory != nil) && (len(sessionHistory) > 0) {
		session.History = sessionHistory
	}

	resp, err := session.SendMessage(context, genai.Text(input))
	helper.PanicIfError("Error saat mengambil respon:", err)

	return string(resp.Candidates[0].Content.Parts[0].(genai.Text))
}

func (service *ProcessServiceImpl) Delete(phone string) bool {

	err := service.DB.Where("phone = ?", phone).Delete(&model.Process{}).Error
	helper.PanicIfError("Error saat menghapus AI model:", err)

	filePath := filepath.Join("session", "wa-"+phone+".db")
	if _, err := os.Stat(filePath); err == nil {
		// Attempt to delete the file
		err = os.Remove(filePath)
		if err != nil {
			helper.PanicIfError("Error saat menghapus file sesi:", err)
		}
	}

	return true
}
