package version

// Name is the user-facing application name.
const Name = "LightScraper"

// These values are overridden at build time with -ldflags, for example:
//
//	-X github.com/dncore/lightscraper/internal/version.Version=0.1.0
//	-X github.com/dncore/lightscraper/internal/version.Commit=<sha>
//	-X github.com/dncore/lightscraper/internal/version.BuildDate=<RFC3339>
var (
	Version   = "0.1.0-dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info is the version information displayed by the application and exposed to
// the frontend through the Wails binding.
type Info struct {
	Name      string
	Version   string
	Commit    string
	BuildDate string
}

// Current returns the version metadata for this build.
func Current() Info {
	return Info{
		Name:      Name,
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
	}
}
