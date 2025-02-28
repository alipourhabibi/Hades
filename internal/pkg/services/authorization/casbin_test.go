package authorization

import (
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/util"
)

func TestMain(t *testing.T) {
	// TODO fix the paths
	c, err := casbin.NewEnforcer("../../../../config/rbac_model.conf", "../../../../config/rbac_model_policy_test.csv")
	if err != nil {
		t.Fatal(err)
	}

	c.AddNamedDomainMatchingFunc("g", "keyMatch2", util.KeyMatch2)

	testCases := []struct {
		user, dom, resource, action string
		expected                    bool
	}{
		{"alice", "alice/something-else", "repository", "create", true},
		{"hades", "hades/hades", "repository", "create", true},

		{"bob", "alice/test", "cicd", "read", true},
		{"bob", "alice/test", "cicd", "write", true},
		{"bob", "alice/test", "cicd", "delete", false},

		{"bob", "alice/cicd", "cicd", "read", false},
		{"bob", "alice/cicd", "cicd", "write", false},
		{"bob", "alice/cicd", "cicd", "delete", false},

		{"charlie", "alice/test", "cicd", "read", false},
		{"charlie", "alice/test", "cicd", "write", false},
		{"charlie", "alice/test", "cicd", "delete", false},

		{"alice", "alice/test", "repository", "read", true},
		{"alice", "alice/test", "repository", "delete", true},
		{"alice", "alice/test", "repository", "write", true},

		{"alice", "alice/cicd", "deploy", "read", true},
		{"alice", "alice/cicd", "deploy", "delete", true},
		{"alice", "alice/cicd", "deploy", "write", true},

		{"bob", "alice/anything", "cicd", "read", false},
		{"bob", "alice/anything", "cicd", "delete", false},
		{"bob", "alice/anything", "cicd", "write", false},

		{"alice", "alice/anything", "cicd", "read", true},
		{"alice", "alice/anything", "cicd", "delete", true},
		{"alice", "alice/anything", "cicd", "write", true},

		{"alice", "bob/anything", "cicd", "read", false},
		{"alice", "bob/anything", "cicd", "write", false},
		{"alice", "bob/anything", "cicd", "delete", false},
	}

	for _, tc := range testCases {
		allowed, err := c.Enforce(tc.user, tc.dom, tc.resource, tc.action)
		if err != nil {
			t.Fatalf("error enforcing policy: %v", err)
		}
		if allowed != tc.expected {
			t.Fatalf("on %v expected %v, got %v", tc, tc.expected, allowed)
		}
	}
}
