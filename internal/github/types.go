package github

import (
	"context"

	"github.com/google/go-github/v50/github"
)

type ListOpts struct {
	RepoOwner     string
	RepoName      string
	Labels        []string
	ExcludeLabels []string
	BaseRef       string
}

type CommentOpts struct {
	RepoOwner string
	RepoName  string
	PrNumber  int
	Message   string
	CommentID *int64
}

type IssueWithTimeline struct {
	Issue       *github.Issue
	PullRequest *github.PullRequest
	Timeline    []*github.Timeline
}

type Client interface {
	ListLabelledOpenPullsWithTimeline(context.Context, *ListOpts) ([]IssueWithTimeline, error)
	CommentPR(context.Context, *CommentOpts) (*github.IssueComment, error)
	RawClient() *github.Client
}
