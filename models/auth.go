package models

import "time"

type JsonModel struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GoogleAuthSignIn struct {
	IdToken  string `json:"idToken" validate:"required"`
	Platform string `json:"platform" validate:"required"`
	// StockCount uint    `json:"stock-count"`
}

type AppleAuthRequest struct {
	IdentityToken     string `json:"identity_token" validate:"required"`
	Platform          string `json:"platform" validate:"required"`
	AuthorizationCode string `json:"authorization_code" validate:"required"`
}

type SignUpIn struct {
	ProfileIn
	IdToken  string `json:"idToken" validate:"required"`
	Platform string `json:"platform" validate:"required"`
	// Name     string `json:"name" validate:"required"`
	// Company  string `json:"company_name" validate:"required"`
	// StockCount uint    `json:"stock-count"`
}

type ProfileIn struct {
	Name      string `json:"name" validate:"required"`
	Company   string `json:"company" validate:"required"`
	UTMSource string `json:"utm_source" validate:"required"`
	// Avatar  string `json:"avatar"`
	// StockCount uint    `json:"stock-count"`
}

type GoogleSignInOut struct {
	Email string `json:"email"`

	// these two null in first step
	Id        string `json:"id"`
	CompanyId string `json:"company_id"`

	New         bool   `json:"new"`
	Avatar      string `json:"avatar"`
	AccessToken string `json:"access_token"`
}

type CompanyInfoRoleV2Out struct {
	CompanyInfoOut
	// Name             string       `json:"name"`
	// Id               uint         `json:"id"`
	// Active           bool         `json:"active"`
	Role string `json:"role"`
	// Subscription     Subscription `json:"subscription"`
	// TrialStartedDate *int64       `json:"trial_started_date"`
	// TrialDays        *uint        `json:"trial_days"`
}

// deprecated
type UserMeInfoOut struct {
	Id                        string           `json:"id"`
	CompanyId                 string           `json:"company_id"`
	Name                      string           `json:"name"`
	MyCompanies               []CompanyInfoOut `json:"my_companies"`
	Email                     string           `json:"email"`
	Status                    string           `json:"-"`
	AvatarURL                 string           `json:"avatar_url"`
	ReceiveSalesNotifications bool             `json:"receive_notifications"`
}

type UserMeInfoV2Out struct {
	Id                                   string                 `json:"id"`
	CompanyId                            string                 `json:"company_id"`
	Name                                 string                 `json:"name"`
	MyCompanies                          []CompanyInfoRoleV2Out `json:"my_companies"`
	Email                                string                 `json:"email"`
	Status                               string                 `json:"-"`
	AvatarURL                            string                 `json:"avatar_url"`
	ReceiveSalesNotifications            bool                   `json:"receive_notifications"`
	FullBodyAvatarUrl                    *string                `json:"user_fullbody_avatar_url"`
	FullBodyAvatarProcessingErrorMessage *string                `json:"full_body_avatar_processing_error_message"`
	FullBodyAvatarSet                    bool                   `json:"full_body_avatar_set"`
	FullBodyAvatarStatus                 string                 `json:"full_body_avatar_status"`
	// Person characteristics
	BodyType       *string `json:"body_type"`
	ShoulderType   *string `json:"shoulder_type"`
	BodyToLegRatio *string `json:"body_to_leg_ratio"`
	HandType       *string `json:"hand_type"`
	UpperLimbType  *string `json:"upper_limb_type"`
	Weight         *int    `json:"weight"`
	Height         *string `json:"height"`
	WaistSize      *int    `json:"waist_size"`
}

type UserInfoOut struct {
	Id          uint   `json:"id"`
	CompanyId   string `json:"company_id"`
	Name        string `json:"name"`
	CompanyName string `json:"company_name"`
	Email       string `json:"email"`
	Status      string `json:"-"`
	AvatarURL   string `json:"avatar_url"`
}

type MemberInfoOut struct {
	// Id          string `json:"id"`
	UserInfo   UserInfoOut `json:"user"`
	Active     bool        `json:"active"`
	Role       Role        `json:"role"`
	InviteCode *string     `json:"invite_code"`
	// CompanyId   string `json:"company_id"`
	// Name        string `json:"name"`
	// CompanyName string `json:"company_name"`
	// Email       string `json:"email"`
	// Status      string `json:"-"`
	// AvatarURL   string `json:"avatar_url"`
}

type CompanyOverviewOut struct {
	Name                   string          `json:"name"`
	Address                *string         `json:"address"`
	LocationPin            *string         `json:"location_pin"`
	BusinessPhone          *string         `json:"business_number"`
	WhatsAppNumber         *string         `json:"whatsapp_number"`
	InstagramURL           *string         `json:"instagram_url"`
	FacebookURL            *string         `json:"facebook_url"`
	TiktokURL              *string         `json:"tiktok_url"`
	ImageUrl               *string         `json:"image_url"`
	Members                []MemberInfoOut `json:"members"`
	Subscription           string          `json:"subscription"`
	OwnerID                uint            `json:"owner_id"`
	Currency               string          `json:"currency"`
	Language               string          `json:"language"`
	TodayCreatedNotesCount *int64          `json:"today_created_notes_count"`
	TotalCreatedNotesCount *int64          `json:"total_created_notes_count"`
	DefaultDailyNoteLimit  int32           `json:"default_daily_note_limit"`
	DefaulTotalNoteLimit   int32           `json:"default_total_note_limit"`
	FullAdminAccess        bool            `json:"full_admin_access"`
	LLMModel               *int32          `json:"llm_model"`
}

type CompanyInfoOut struct {
	Name                     string       `json:"name"`
	Subscription             Subscription `json:"subscription"`
	OwnerId                  uint         `json:"owner_id"`
	SalesAllowManageProducts bool         `json:"sales_allow_manage_products"`
	SalesAllowViewDashboard  bool         `json:"sales_allow_view_dashboard"`
	Id                       uint         `json:"id"`
	Active                   bool         `json:"active"`
	TrialStartedDate         *int64       `json:"trial_started_date"`
	TrialDays                *uint        `json:"trial_days"`
	FullAdminAccess          bool         `json:"full_admin_access"`
}

type CompanyUpdateIn struct {
	Name     *string `json:"name"`
	LLMModel *int32  `json:"llm_model"`
	Language *string `json:"language"`
}

type MemberAddIn struct {
	Email string `json:"email"`
	Role  Role   `json:"role"`
}

type PersonCharacteristicsIn struct {
	BodyType       *string  `json:"body_type" validate:"omitempty,oneof=slender athletic robust"`
	ShoulderType   *string  `json:"shoulder_type" validate:"omitempty,oneof=narrow proportionate broad"`
	BodyToLegRatio *string  `json:"body_to_leg_ratio" validate:"omitempty,oneof=long_legs balanced long_torso"`
	HandType       *string  `json:"hand_type" validate:"omitempty,oneof=slender proportioned large"`
	UpperLimbType  *string  `json:"upper_limb_type" validate:"omitempty,oneof=slender toned muscular"`
	Weight         *int     `json:"weight" validate:"omitempty,min=30,max=300"`
	Height         *float64 `json:"height" validate:"omitempty,min=1.0,max=2.5"`
	WaistSize      *int     `json:"waist_size" validate:"omitempty,min=50,max=200"`
}
