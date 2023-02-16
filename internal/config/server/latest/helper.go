package latest

import "github.com/rs/zerolog"

func (r GithubRepositoryConfig) MarshalZerologObject(e *zerolog.Event) {
	e.Str("gh_repo_owner", r.Owner).
		Str("gh_repo_name", r.Name).
		Str("gh_base_ref", r.BaseRef)
}
