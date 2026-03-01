package providers

type ProviderRegistry struct {
	providers   map[string]LLMProvider
	defaultName string
}

func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers:   make(map[string]LLMProvider),
		defaultName: "",
	}
}

func (r *ProviderRegistry) Register(name string, p LLMProvider) {
	r.providers[name] = p

	if r.defaultName == "" {
		r.defaultName = name
	}
}

func (r *ProviderRegistry) Get(name string) (LLMProvider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

func (r *ProviderRegistry) SetDefault(name string) bool {
	if _, ok := r.providers[name]; !ok {
		return false
	}
	r.defaultName = name
	return true
}
func (r *ProviderRegistry) GetDefault() (LLMProvider, bool) {
	if r.defaultName == "" {
		return nil, false
	}
	p, ok := r.providers[r.defaultName]
	return p, ok
}

func (r *ProviderRegistry) List() []string {
	keys := make([]string, 0, len(r.providers))
	for k := range r.providers {
		keys = append(keys, k)
	}
	return keys
}
