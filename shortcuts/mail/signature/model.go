// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signature

// SignatureType represents the type of a mail signature.
type SignatureType string

const (
	SignatureTypeUser   SignatureType = "USER"
	SignatureTypeTenant SignatureType = "TENANT"
)

// SignatureDevice represents the device platform a signature is designed for.
type SignatureDevice string

const (
	DevicePC     SignatureDevice = "PC"
	DeviceMobile SignatureDevice = "MOBILE"
)

// SignatureImage holds metadata for an inline image embedded in a signature.
type SignatureImage struct {
	ImageName   string `json:"image_name,omitempty"`
	FileKey     string `json:"file_key,omitempty"`
	CID         string `json:"cid,omitempty"`
	FileSize    string `json:"file_size,omitempty"`
	Header      string `json:"header,omitempty"`
	ImageWidth  int32  `json:"image_width,omitempty"`
	ImageHeight int32  `json:"image_height,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
}

// UserFieldValue holds a template variable value with multi-language support.
type UserFieldValue struct {
	DefaultVal string            `json:"default_val"`
	I18nVals   map[string]string `json:"i18n_vals"` // keys: "zh_cn", "en_us", "ja_jp"
}

// Resolve returns the localized value for the given language code.
// Falls back to DefaultVal when the language key is missing or empty.
func (v UserFieldValue) Resolve(lang string) string {
	if val, ok := v.I18nVals[lang]; ok && val != "" {
		return val
	}
	return v.DefaultVal
}

// Signature represents a single mail signature returned by the API.
type Signature struct {
	ID               string                    `json:"id"`
	Name             string                    `json:"name"`
	SignatureType    SignatureType             `json:"signature_type"`
	SignatureDevice  SignatureDevice           `json:"signature_device"`
	Content          string                    `json:"content"`
	Images           []SignatureImage          `json:"images,omitempty"`
	TemplateJSONKeys []string                  `json:"template_json_keys,omitempty"`
	UserFields       map[string]UserFieldValue `json:"user_fields,omitempty"`
}

// IsTenant returns true if this is a tenant/corporate signature with template variables.
func (s *Signature) IsTenant() bool {
	return s.SignatureType == SignatureTypeTenant
}

// HasTemplateVars returns true if the signature contains template variables that need interpolation.
func (s *Signature) HasTemplateVars() bool {
	return len(s.TemplateJSONKeys) > 0
}

// SignatureUsage indicates which signature is used by default for a given email address.
type SignatureUsage struct {
	EmailAddress        string `json:"email_address"`
	SendMailSignatureID string `json:"send_mail_signature_id"`
	ReplySignatureID    string `json:"reply_signature_id"`
}

// GetSignaturesResponse is the parsed response from the get_signatures API.
type GetSignaturesResponse struct {
	Signatures []Signature      `json:"signatures"`
	Usages     []SignatureUsage `json:"usages"`
}
