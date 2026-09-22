package gormpaginate

type config struct {
	allowFilters []string
	allowSorts   []string
	defaultSort  *sortSpec
}

type sortSpec struct {
	column    string
	direction string
}

type Option func(*config)

func newConfig(opts ...Option) *config {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// AllowFilters restricts which JSON columns can be used in filters.
// If not specified, all columns discoverable in the GORM schema are allowed.
func AllowFilters(columns ...string) Option {
	return func(c *config) { c.allowFilters = columns }
}

// AllowSorts restricts which JSON columns can be used for sorting.
// If not specified, all columns discoverable in the GORM schema are allowed.
func AllowSorts(columns ...string) Option {
	return func(c *config) { c.allowSorts = columns }
}

// DefaultSort sets a default sort column (JSON key) and direction ("ASC"/"DESC") when none are specified.
func DefaultSort(column, direction string) Option {
	return func(c *config) { c.defaultSort = &sortSpec{column, direction} }
}
