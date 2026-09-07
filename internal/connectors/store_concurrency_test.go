package connectors

import (
	"path/filepath"
	"testing"

	"csgclaw/internal/auth"
)

func TestStoreConcurrentProviderSavesPreserveBoth(t *testing.T) {
	for round := range 32 {
		path := filepath.Join(t.TempDir(), "state.json")
		// Separate store instances must coordinate through the shared state owner.
		githubStore, gitlabStore := NewStore(path), NewStore(path)
		runConcurrentStoreChanges(t,
			func() error { return githubStore.SaveGitHub(State{Config: Config{ClientID: "test-github-id"}}) },
			func() error {
				return gitlabStore.SaveGitLab(State{Config: Config{BaseURL: "https://gitlab.example", AccessToken: "test-gitlab-token"}})
			},
		)
		github, foundGitHub, err := githubStore.LoadGitHub()
		if err != nil {
			t.Fatal(err)
		}
		gitlab, foundGitLab, err := gitlabStore.LoadGitLab()
		if err != nil {
			t.Fatal(err)
		}
		if !foundGitHub || !foundGitLab || github.Config.ClientID != "test-github-id" || gitlab.Config.AccessToken != "test-gitlab-token" {
			t.Fatalf("round %d: concurrent saves lost provider data, github=%v, gitlab=%v", round, foundGitHub, foundGitLab)
		}
	}
}

func TestStoreConcurrentProviderSaveAndDeletePreserveBothChanges(t *testing.T) {
	for _, deleteAccount := range []bool{false, true} {
		name := "delete_connector"
		if deleteAccount {
			name = "delete_account"
		}
		t.Run(name, func(t *testing.T) {
			for round := range 32 {
				path := filepath.Join(t.TempDir(), "state.json")
				connectorStore, authStore := NewStore(path), auth.NewStore(path)
				account := auth.Record{Tokens: auth.Tokens{AccessToken: "test-old-account-token"}, Account: auth.Account{BaseURL: "https://account.example"}}
				if err := authStore.Save(account); err != nil {
					t.Fatal(err)
				}
				if err := connectorStore.SaveGitHub(State{Config: Config{ClientID: "test-old-github-id"}}); err != nil {
					t.Fatal(err)
				}
				if deleteAccount {
					runConcurrentStoreChanges(t, authStore.Delete, func() error {
						return connectorStore.SaveGitHub(State{Config: Config{ClientID: "test-new-github-id"}})
					})
				} else {
					account.Tokens.AccessToken = "test-new-account-token"
					runConcurrentStoreChanges(t, connectorStore.DeleteGitHub, func() error { return authStore.Save(account) })
				}
				github, foundGitHub, err := connectorStore.LoadGitHub()
				if err != nil {
					t.Fatal(err)
				}
				storedAccount, foundAccount, err := authStore.Load()
				if err != nil {
					t.Fatal(err)
				}
				if deleteAccount {
					if foundAccount || !foundGitHub || github.Config.ClientID != "test-new-github-id" {
						t.Fatalf("round %d: account deletion or connector update was lost", round)
					}
				} else if foundGitHub || !foundAccount || storedAccount.Tokens.AccessToken != "test-new-account-token" {
					t.Fatalf("round %d: connector deletion or account update was lost", round)
				}
			}
		})
	}
}

func runConcurrentStoreChanges(t *testing.T, changes ...func() error) {
	t.Helper()
	start := make(chan struct{})
	results := make(chan error, len(changes))
	for _, change := range changes {
		go func() {
			<-start
			results <- change()
		}()
	}
	close(start)
	for range changes {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
}
