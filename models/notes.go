package models

import (
	"time"

	"github.com/lib/pq"
)

type Note struct {
	JsonModel
	Name string `json:"name"`

	NoteType      *string `json:"note_type"`
	Transcript    *string `gorm:"type:text" json:"transcript"`
	ConspectAIMD5 *string `json:"conspect_ai_md5"`
	// user enforced language - not LLM detected
	LanguageCode *string `json:"language_code"`
	// TODO: teachers should be able to create tests!!
	// TODO: to audio anything for user
	QuizJSON       *string `gorm:"type:text" json:"quiz_json"`
	FlashcardsJSON *string `gorm:"type:text" json:"flashcards_json"`
	// this is file **key** in storage.
	FileUrl       *string     `json:"file_url"`
	FileName      *string     `json:"file_name"`
	ImageUrl      *string     `json:"image_url"`
	YoutubeUrl    *string     `json:"youtube_url"`
	YoutubeId     *string     `json:"youtube_id"`
	TotalDuration *float64    `json:"total_duration"`
	PDFUrl        *string     `json:"pdf_url"`
	AudioUrl      *string     `json:"audio_url"`
	Owner         UserAccount `json:"-"`
	OwnerID       uint        `json:"-"`
	CompanyID     uint        `json:"-"`
	Company       Company     `json:"company"`
	Folder        *Folder     `json:"folder"`
	FolderID      *uint       `json:"folder_id"`
	Language      string      `json:"language"`
	Deleted       bool        `json:"deleted"`
	Status        string      `json:"status"`
	// generated, ready_to_generate, failed
	QuizAlertsEnabled          bool    `json:"quiz_alerts_enabled"`
	QuizStatus                 string  `json:"quiz_status"`
	ProcessingErrorMessage     *string `json:"processing_error_message"`
	ProcessingQuizErrorMessage *string `json:"processing_quiz_error_message"`
	ProcessRetryTimes          uint    `json:"-"`
	QuizProcessRetryTimes      uint    `json:"-"`
	ModelPreference            *string `json:"-"`

	InputTokenCount        *int32  `json:"prompt_token_count"`
	ThoughtsTokenCount     *int32  `json:"thoughts_token_count"`
	OuputTokenCount        *int32  `json:"output_token_count"`
	TotalTokenCount        *int32  `json:"total_token_count"`
	Thoughts               *string `gorm:"type:text" json:"thoughts"`
	LLMModel               *string `json:"llm_model"`
	QuizInputTokenCount    *int32  `json:"quiz_prompt_token_count"`
	QuizThoughtsTokenCount *int32  `json:"quiz_thoughts_token_count"`
	QuizOuputTokenCount    *int32  `json:"quiz_output_token_count"`
	QuizTotalTokenCount    *int32  `json:"quiz_total_token_count"`
	QuizThoughts           *string `gorm:"type:text" json:"quiz_thoughts"`
	QuizLLMModel           *string `json:"quiz_model"`

	QuestionGenerationMetadata *string `gorm:"type:text" json:"question_generation_metadata"`
	QuestionGeneratedCount     uint    `json:"question_generated_count"`
	// Questions
	Questions                      []Question `json:"questions"`
	FailedToProcessLLMNoteResponse string     `gorm:"type:text" json:"-"`
	AlertWhenProcessed             bool       `json:"alert_when_processed"`
	// TotalLLMTokens uint        `json:"total_llm_tokens"`
}

type Folder struct {
	JsonModel
	Name    string      `json:"name"`
	Owner   UserAccount `json:"-"`
	OwnerID uint        `json:"-"`
	Notes   []Note      `json:"notes"`
}

type Question struct {
	JsonModel
	NoteID uint `json:"-"`
	Note   Note `gorm:"constraint:OnDelete:CASCADE;" json:"-"`
	//type:ENUM('easy', 'medium', 'hard')
	ComplexityLevel string `json:"complexity_level"`
	// type:ENUM('multiple_choice', 'single_choice', 'fill_in_the_blank')"
	Type             string         `json:"type"`
	QuestionText     string         `gorm:"type:text;not null" json:"question_text"`
	Explanation      string         `gorm:"type:text;" json:"explanation"`
	Options          pq.StringArray `gorm:"type:text[];not null" json:"options"`
	Answer           string         `gorm:"type:text;not null" json:"answer"`
	UserAnswer       string         `gorm:"type:text;not null" json:"user_answer"`
	UserAnsweredDate *time.Time     `json:"user_answered_date"`
}
