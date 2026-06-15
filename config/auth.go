package config

// AuthConfig configures authentication subsystems: passwords, sessions,
// lockout behaviour, email verification, and password reset.
type AuthConfig struct {
	Password          PasswordConfig     `json:"password" yaml:"password"`
	Session           SessionConfig      `json:"session" yaml:"session"`
	Lockout           LockoutConfig      `json:"lockout" yaml:"lockout"`
	EmailVerification EmailVerifConfig   `json:"emailVerification" yaml:"emailVerification"`
	PasswordReset     PasswordResetConfig `json:"passwordReset" yaml:"passwordReset"`
}

type PasswordConfig struct {
	MinLength  int `json:"minLength" yaml:"minLength"`
	BcryptCost int `json:"bcryptCost" yaml:"bcryptCost"`
}

type SessionConfig struct {
	IdleTimeoutDays      int `json:"idleTimeoutDays" yaml:"idleTimeoutDays"`
	AbsoluteTimeoutDays  int `json:"absoluteTimeoutDays" yaml:"absoluteTimeoutDays"`
	RotationGraceSeconds int `json:"rotationGraceSeconds" yaml:"rotationGraceSeconds"`
}

type LockoutConfig struct {
	MaxAttempts     int `json:"maxAttempts" yaml:"maxAttempts"`
	CooldownMinutes int `json:"cooldownMinutes" yaml:"cooldownMinutes"`
}

type EmailVerifConfig struct {
	TokenExpiryHours int `json:"tokenExpiryHours" yaml:"tokenExpiryHours"`
}

type PasswordResetConfig struct {
	TokenExpiryHours int `json:"tokenExpiryHours" yaml:"tokenExpiryHours"`
}

type EmailConfig struct {
	Stub bool       `json:"stub" yaml:"stub"`
	From string     `json:"from" yaml:"from"`
	SMTP SMTPConfig `json:"smtp" yaml:"smtp"`
}

type SMTPConfig struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
}

type OAuthConfig struct {
	GitHub OAuthProvider `json:"github" yaml:"github"`
	Google OAuthProvider `json:"google" yaml:"google"`
}

type OAuthProvider struct {
	ClientID     string `json:"clientId" yaml:"clientId"`
	ClientSecret string `json:"clientSecret" yaml:"clientSecret"`
	RedirectURL  string `json:"redirectUrl" yaml:"redirectUrl"`
}

type RedisConfig struct {
	Addr     string `json:"addr" yaml:"addr"`
	Password string `json:"password" yaml:"password"`
	DB       int    `json:"db" yaml:"db"`
}

// TOTPConfig configures time-based one-time password (TOTP) two-factor authentication.
type TOTPConfig struct {
	EncryptionKey string `json:"encryptionKey" yaml:"encryptionKey"` // 32-byte hex AES-256 key; encrypts TOTP secrets at rest
	Issuer        string `json:"issuer" yaml:"issuer"`
}
