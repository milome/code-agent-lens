package dump

type Config struct {
	Enabled              bool
	Root                 string
	RunID                string
	CaptureHeaders       string
	CaptureBodies        string
	CaptureSecrets       bool
	CaptureStreamEvents  string
	MaxBodyBytes         int64
	PromptExtractEnabled bool
}
