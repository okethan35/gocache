type Command struct {
	Op    string
	Key   string
	Value string // only meaningful for SET
	TTL   int    // only meaningful for SET, -1 if not provided
}

var ErrInvalidCommand = errors.New("invalid command")

func ParseCommand(line string) (Command, error)