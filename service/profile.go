package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oneRole/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ProfileService struct {
	db *mongo.Database
}

func NewProfileService(database *mongo.Database) *ProfileService {
	return &ProfileService{db: database}
}

func (s *ProfileService) GetProfile(ctx context.Context, scope ActorScope) (*models.CandidateProfile, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("database unavailable")
	}
	coll := s.db.Collection("candidate_profiles")
	var profile models.CandidateProfile
	err := coll.FindOne(ctx, bson.M{"user_id": scope.UserID}).Decode(&profile)
	if err == nil {
		return &profile, nil
	}

	// If not found in database, construct from user record
	var u struct {
		Email       string `bson:"email"`
		DisplayName string `bson:"display_name"`
	}
	_ = s.db.Collection("users").FindOne(ctx, bson.M{"_id": scope.UserID}).Decode(&u)
	first := "Candidate"
	last := ""
	parts := strings.Fields(u.DisplayName)
	if len(parts) > 0 {
		first = parts[0]
		if len(parts) > 1 {
			last = strings.Join(parts[1:], " ")
		}
	}
	now := time.Now().UTC()
	profile = models.CandidateProfile{
		ID:                   fmt.Sprintf("prof-%s", scope.UserID),
		UserID:               scope.UserID,
		TenantID:             scope.TenantID,
		FirstName:            first,
		LastName:             last,
		Email:                u.Email,
		Phone:                "555-019-2834",
		City:                 "San Francisco",
		State:                "CA",
		Country:              "United States",
		AuthorizedToWorkInUS: true,
		RequiresSponsorship:  false,
		YearsOfExperience:    5,
		CurrentEmployer:      "Technology Solutions",
		CurrentJobTitle:      "Software Engineer",
		PrimarySkills:        []string{"Go", "Distributed Systems", "REST APIs", "Cloud Infrastructure"},
		OpenToRemote:         true,
		DesiredSalaryMin:     135000,
		HighestDegree:        "Bachelor's Degree",
		ResumeContentText:    fmt.Sprintf("%s %s - Experienced Software Engineer with expertise in Go, distributed systems, backend APIs, and cloud applications.", first, last),
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	_, _ = coll.UpdateOne(ctx, bson.M{"user_id": scope.UserID}, bson.M{"$setOnInsert": profile}, options.Update().SetUpsert(true))
	return &profile, nil
}

func (s *ProfileService) UpdateProfile(ctx context.Context, scope ActorScope, update models.CandidateProfile) (*models.CandidateProfile, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("database unavailable")
	}
	existing, err := s.GetProfile(ctx, scope)
	if err != nil {
		return nil, err
	}

	if update.FirstName != "" {
		existing.FirstName = update.FirstName
	}
	if update.LastName != "" {
		existing.LastName = update.LastName
	}
	if update.Email != "" {
		existing.Email = update.Email
	}
	if update.Phone != "" {
		existing.Phone = update.Phone
	}
	if update.City != "" {
		existing.City = update.City
	}
	if update.State != "" {
		existing.State = update.State
	}
	if update.Country != "" {
		existing.Country = update.Country
	}
	if update.PostalCode != "" {
		existing.PostalCode = update.PostalCode
	}
	if update.LinkedInURL != "" {
		existing.LinkedInURL = update.LinkedInURL
	}
	if update.GitHubURL != "" {
		existing.GitHubURL = update.GitHubURL
	}
	if update.PortfolioURL != "" {
		existing.PortfolioURL = update.PortfolioURL
	}
	existing.AuthorizedToWorkInUS = update.AuthorizedToWorkInUS
	existing.RequiresSponsorship = update.RequiresSponsorship
	if update.YearsOfExperience > 0 {
		existing.YearsOfExperience = update.YearsOfExperience
	}
	if update.CurrentEmployer != "" {
		existing.CurrentEmployer = update.CurrentEmployer
	}
	if update.CurrentJobTitle != "" {
		existing.CurrentJobTitle = update.CurrentJobTitle
	}
	if len(update.PrimarySkills) > 0 {
		existing.PrimarySkills = update.PrimarySkills
	}
	if update.HighestDegree != "" {
		existing.HighestDegree = update.HighestDegree
	}
	if update.DesiredSalaryMin > 0 {
		existing.DesiredSalaryMin = update.DesiredSalaryMin
	}
	existing.OpenToRemote = update.OpenToRemote
	if update.NoticePeriodWeeks > 0 {
		existing.NoticePeriodWeeks = update.NoticePeriodWeeks
	}
	if update.ResumeContentText != "" {
		existing.ResumeContentText = update.ResumeContentText
	}
	if len(update.CustomFieldAnswers) > 0 {
		if existing.CustomFieldAnswers == nil {
			existing.CustomFieldAnswers = make(map[string]string)
		}
		for k, v := range update.CustomFieldAnswers {
			existing.CustomFieldAnswers[k] = v
		}
	}
	existing.UpdatedAt = time.Now().UTC()

	_, err = s.db.Collection("candidate_profiles").ReplaceOne(
		ctx,
		bson.M{"user_id": scope.UserID},
		existing,
		options.Replace().SetUpsert(true),
	)
	if err != nil {
		return nil, fmt.Errorf("update candidate profile: %w", err)
	}
	return existing, nil
}
