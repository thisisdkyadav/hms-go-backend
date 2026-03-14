package auth

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	ID                    primitive.ObjectID `bson:"_id"`
	Name                  string             `bson:"name"`
	Email                 string             `bson:"email"`
	Phone                 string             `bson:"phone,omitempty"`
	ProfileImage          string             `bson:"profileImage,omitempty"`
	Role                  string             `bson:"role"`
	SubRole               *string            `bson:"subRole,omitempty"`
	Authz                 UserAuthz          `bson:"authz,omitempty"`
	PinnedTabs            []string           `bson:"pinnedTabs,omitempty"`
	Password              *string            `bson:"password,omitempty"`
	AESKey                string             `bson:"aesKey,omitempty"`
	AcceptingAppointments bool               `bson:"acceptingAppointments,omitempty"`
	CreatedAt             time.Time          `bson:"createdAt,omitempty"`
	UpdatedAt             time.Time          `bson:"updatedAt,omitempty"`
}

type UserAuthz struct {
	Override UserAuthzOverride `bson:"override,omitempty" json:"override"`
	Meta     *UserAuthzMeta    `bson:"meta,omitempty" json:"meta,omitempty"`
}

type UserAuthzOverride struct {
	AllowRoutes       []string             `bson:"allowRoutes,omitempty" json:"allowRoutes"`
	DenyRoutes        []string             `bson:"denyRoutes,omitempty" json:"denyRoutes"`
	AllowCapabilities []string             `bson:"allowCapabilities,omitempty" json:"allowCapabilities"`
	DenyCapabilities  []string             `bson:"denyCapabilities,omitempty" json:"denyCapabilities"`
	Constraints       []UserAuthzConstraint `bson:"constraints,omitempty" json:"constraints"`
}

type UserAuthzConstraint struct {
	Key   string `bson:"key" json:"key"`
	Value any    `bson:"value" json:"value"`
}

type UserAuthzMeta struct {
	Version   int                 `bson:"version,omitempty" json:"version,omitempty"`
	UpdatedBy *primitive.ObjectID `bson:"updatedBy,omitempty" json:"updatedBy,omitempty"`
	UpdatedAt *time.Time          `bson:"updatedAt,omitempty" json:"updatedAt,omitempty"`
}

type EffectiveAuthz struct {
	CatalogVersion int             `json:"catalogVersion"`
	Role           string          `json:"role"`
	RouteAccess    map[string]bool `json:"routeAccess"`
	Capabilities   map[string]bool `json:"capabilities"`
	Constraints    map[string]any  `json:"constraints"`
}

type UserAuthzResponse struct {
	Override  UserAuthzOverride `json:"override"`
	Effective EffectiveAuthz    `json:"effective"`
	Meta      *UserAuthzMeta    `json:"meta,omitempty"`
}

type HostelSummary struct {
	ID   primitive.ObjectID `bson:"_id"`
	Name string             `bson:"name"`
	Type string             `bson:"type"`
}

type UserResponse struct {
	ID                    string            `json:"_id"`
	Name                  string            `json:"name"`
	Email                 string            `json:"email"`
	Phone                 string            `json:"phone,omitempty"`
	ProfileImage          string            `json:"profileImage,omitempty"`
	Role                  string            `json:"role"`
	SubRole               *string           `json:"subRole"`
	Authz                 UserAuthzResponse `json:"authz"`
	PinnedTabs            []string          `json:"pinnedTabs"`
	AESKey                string            `json:"aesKey"`
	AcceptingAppointments bool              `json:"acceptingAppointments"`
	CreatedAt             time.Time         `json:"createdAt"`
	UpdatedAt             time.Time         `json:"updatedAt"`
	Hostel                *HostelDTO        `json:"hostel"`
}

type HostelDTO struct {
	ID   string `json:"_id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type PasswordResetToken struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	UserID    primitive.ObjectID `bson:"userId"`
	Token     string             `bson:"token"`
	ExpiresAt time.Time          `bson:"expiresAt"`
	Used      bool               `bson:"used"`
	CreatedAt time.Time          `bson:"createdAt"`
}

type PasswordResetPreview struct {
	User ResetTokenUser `json:"user"`
}

type ResetTokenUser struct {
	ID    string `json:"_id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type AuthSessionResult struct {
	User      UserResponse
	SessionID string
	Message   string
}

type VerifiedSSOUser struct {
	ID         string     `json:"_id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	SubRole    *string    `json:"subRole"`
	Hostel     *HostelDTO `json:"hostel"`
	PinnedTabs []string   `json:"pinnedTabs"`
}
