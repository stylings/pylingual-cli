package job

// Job describes one input bytecode file and its planned Python output path.
type Job struct {
	ID         int
	InputPath  string
	OutputPath string
}
