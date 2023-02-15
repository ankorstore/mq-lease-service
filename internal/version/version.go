package version

var (
	name   = ""
	commit = ""
	date   = ""
	tag    = ""
)

type Version struct{}

func (Version) GetAppName() string {
	return name
}

// GetCommit returns the current commit.
func (Version) GetCommit() string {
	return commit
}

// GetTag returns the current commit.
func (Version) GetTag() string {
	return tag
}

// GetBuildDate returns the build date.
func (Version) GetBuildDate() string {
	return date
}
