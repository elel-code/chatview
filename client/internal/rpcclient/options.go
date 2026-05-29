package rpcclient

type Options struct {
	Target                string
	UseTLS                bool
	CACertPath            string
	SSLTargetNameOverride string
}
