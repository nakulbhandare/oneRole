package models

import "time"

// CandidateProfile represents verified factual data used to answer job application questions.
type CandidateProfile struct {
	ID                     string            `bson:"_id" json:"id"`
	UserID                 string            `bson:"user_id" json:"user_id"`
	TenantID               string            `bson:"tenant_id" json:"tenant_id"`
	FirstName              string            `bson:"first_name" json:"first_name"`
	LastName               string            `bson:"last_name" json:"last_name"`
	Email                  string            `bson:"email" json:"email"`
	Phone                  string            `bson:"phone" json:"phone"`
	City                   string            `bson:"city" json:"city"`
	State                  string            `bson:"state" json:"state"`
	Country                string            `bson:"country" json:"country"`
	PostalCode             string            `bson:"postal_code" json:"postal_code"`
	LinkedInURL            string            `bson:"linkedin_url" json:"linkedin_url"`
	GitHubURL              string            `bson:"github_url" json:"github_url"`
	PortfolioURL           string            `bson:"portfolio_url" json:"portfolio_url"`
	AuthorizedToWorkInUS   bool              `bson:"authorized_to_work_in_us" json:"authorized_to_work_in_us"`
	RequiresSponsorship    bool              `bson:"requires_sponsorship" json:"requires_sponsorship"`
	YearsOfExperience      int               `bson:"years_of_experience" json:"years_of_experience"`
	CurrentEmployer        string            `bson:"current_employer" json:"current_employer"`
	CurrentJobTitle        string            `bson:"current_job_title" json:"current_job_title"`
	PrimarySkills          []string          `bson:"primary_skills" json:"primary_skills"`
	HighestDegree          string            `bson:"highest_degree" json:"highest_degree"`
	DesiredSalaryMin       int               `bson:"desired_salary_min" json:"desired_salary_min"`
	OpenToRemote           bool              `bson:"open_to_remote" json:"open_to_remote"`
	NoticePeriodWeeks      int               `bson:"notice_period_weeks" json:"notice_period_weeks"`
	ResumeContentText      string            `bson:"resume_content_text" json:"resume_content_text"`
	ResumeContentHTML      string            `bson:"resume_content_html" json:"resume_content_html"`
	ResumeFilePath         string            `bson:"resume_file_path,omitempty" json:"resume_file_path,omitempty"`
	ResumeFileName         string            `bson:"resume_file_name,omitempty" json:"resume_file_name,omitempty"`
	CustomFieldAnswers     map[string]string `bson:"custom_field_answers" json:"custom_field_answers"`
	ApplicationEmail       string            `bson:"application_email,omitempty" json:"application_email,omitempty"`
	EmailRoutingAlias      string            `bson:"email_routing_alias,omitempty" json:"email_routing_alias,omitempty"`
	SubmissionMode         string            `bson:"submission_mode,omitempty" json:"submission_mode,omitempty"`
	CreatedAt              time.Time         `bson:"created_at" json:"created_at"`
	UpdatedAt              time.Time         `bson:"updated_at" json:"updated_at"`
}
