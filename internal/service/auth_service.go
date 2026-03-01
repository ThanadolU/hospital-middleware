package service

import (
	"errors"

	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/repository"
	"github.com/ThanadolU/hospital-middleware/pkg/utils"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AuthService interface {
	Register(username, password string, hospitalID uuid.UUID) error
	Login(username, password string, hospital uuid.UUID) (string, error)
}

type authService struct {
	repo repository.StaffRepository
}

func NewAuthService(repo repository.StaffRepository) AuthService {
	return &authService{repo: repo}
}

func (a *authService) Register(username, password string, hospitalID uuid.UUID) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	staff := &models.Staff{
		Username: username,
		Password: string(hashed),
		HospitalID: hospitalID,
	}

	return a.repo.Create(staff)
}

func (a *authService) Login(username, password string, hospitalID uuid.UUID) (string, error) {
	staff, err := a.repo.FindByUsernameAndHospital(username, hospitalID)
	if err != nil {
		return "", errors.New("invalid credentials")
	}

	if bcrypt.CompareHashAndPassword([]byte(staff.Password), []byte(password)) != nil {
		return "", errors.New("invalid credentials")
	}

	return utils.GenerateToken(staff.ID, staff.Username, staff.HospitalID)
}
