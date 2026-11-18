package models

type ScreeningQuestion struct {
	ID        string `bson:"_id"`
	CompanyID string `bson:"company_id"`
	Question  string `bson:"question"`
	Pattern   string `bson:"pattern"` // Regex for matching
}

type UserAnswer struct {
	UserID     string `bson:"user_id"`
	QuestionID string `bson:"question_id"`
	Answer     string `bson:"answer"`
}
