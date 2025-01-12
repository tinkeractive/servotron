package servotron

// NOTE if route json changes, then the route struct must change
type Route struct {
	Name        string
	Type        string
	URLScheme   string
	QueryParams []string
	ServiceURL  string
	Description string
}
