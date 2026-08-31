package config

// S3Config describes an S3-compatible object storage service.
type S3Config struct {
	Endpoint          string `mapstructure:"endpoint"`
	Region            string `mapstructure:"region"`
	Bucket            string `mapstructure:"bucket"`
	AccessKeyID       string `mapstructure:"access_key_id"`
	SecretAccessKey   string `mapstructure:"secret_access_key"`
	SessionToken      string `mapstructure:"session_token"`
	ForcePathStyle    bool   `mapstructure:"force_path_style"`
	PresignExpiryMins int    `mapstructure:"presign_expiry_mins"`
	MaxUploadSize     int64  `mapstructure:"max_upload_size_bytes"`
}
