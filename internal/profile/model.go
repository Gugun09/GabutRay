package profile

type Protocol string

const (
	ProtocolVLESS  Protocol = "vless"
	ProtocolVMess  Protocol = "vmess"
	ProtocolTrojan Protocol = "trojan"
)

type Profile struct {
	ID            string            `json:"id" yaml:"id"`
	Name          string            `json:"name" yaml:"name"`
	Protocol      Protocol          `json:"protocol" yaml:"protocol"`
	Address       string            `json:"address" yaml:"address"`
	Port          int               `json:"port" yaml:"port"`
	Auth          Auth              `json:"auth" yaml:"auth"`
	Security      SecuritySettings  `json:"security" yaml:"security"`
	Transport     TransportSettings `json:"transport" yaml:"transport"`
	RawLink       string            `json:"raw_link" yaml:"raw_link"`
	CreatedAtUnix int64             `json:"created_at_unix" yaml:"created_at_unix"`
}

type Auth struct {
	ID         string `json:"id,omitempty" yaml:"id,omitempty"`
	Password   string `json:"password,omitempty" yaml:"password,omitempty"`
	AlterID    int    `json:"alter_id,omitempty" yaml:"alter_id,omitempty"`
	Security   string `json:"security,omitempty" yaml:"security,omitempty"`
	Encryption string `json:"encryption,omitempty" yaml:"encryption,omitempty"`
	Flow       string `json:"flow,omitempty" yaml:"flow,omitempty"`
}

type SecuritySettings struct {
	Kind          string   `json:"kind" yaml:"kind"`
	SNI           string   `json:"sni,omitempty" yaml:"sni,omitempty"`
	Fingerprint   string   `json:"fingerprint,omitempty" yaml:"fingerprint,omitempty"`
	PublicKey     string   `json:"public_key,omitempty" yaml:"public_key,omitempty"`
	ShortID       string   `json:"short_id,omitempty" yaml:"short_id,omitempty"`
	SpiderX       string   `json:"spider_x,omitempty" yaml:"spider_x,omitempty"`
	AllowInsecure bool     `json:"allow_insecure" yaml:"allow_insecure"`
	ALPN          []string `json:"alpn,omitempty" yaml:"alpn,omitempty"`
}

type TransportSettings struct {
	Network     string `json:"network" yaml:"network"`
	Host        string `json:"host,omitempty" yaml:"host,omitempty"`
	Path        string `json:"path,omitempty" yaml:"path,omitempty"`
	ServiceName string `json:"service_name,omitempty" yaml:"service_name,omitempty"`
	Mode        string `json:"mode,omitempty" yaml:"mode,omitempty"`
	HeaderType  string `json:"header_type,omitempty" yaml:"header_type,omitempty"`
}

func NoneSecurity() SecuritySettings {
	return SecuritySettings{Kind: "none"}
}
