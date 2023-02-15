package github

import (
	"context"

	"github.com/google/go-github/v50/github"
	"golang.org/x/oauth2"
)

// NewPatClient connects to GitHub using a personal access token.
func NewPatClient(ctx context.Context, pat string) (*github.Client, error) {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: pat},
	)
	tc := oauth2.NewClient(ctx, ts)

	ghClient := github.NewClient(tc)

	return ghClient, nil
}
