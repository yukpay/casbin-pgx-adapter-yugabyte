package pgxadapter

import (
	"context"
	"os"
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/util"
	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dropDB(t *testing.T, dbname string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("PG_CONN"))
	require.NoError(t, err)
	_, err = conn.Exec(ctx, "DROP DATABASE "+dbname)
	require.NoError(t, err)
	require.NoError(t, conn.Close(ctx))
}

func assertPolicy(t *testing.T, expected, res [][]string) {
	t.Helper()
	assert.True(t, util.Array2DEquals(expected, res), "Policy Got: %v, supposed to be %v", res, expected)
}

func testSaveLoad(t *testing.T, a *Adapter, e *casbin.Enforcer) {
	assert.False(t, e.IsFiltered())
	assertPolicy(t,
		[][]string{{"alice", "data1", "read"}, {"bob", "data2", "write"}, {"data2_admin", "data2", "read"}, {"data2_admin", "data2", "write"}},
		e.GetPolicy(),
	)
}

func testAutoSave(t *testing.T, a *Adapter, e *casbin.Enforcer) {
	// AutoSave is enabled by default.
	// Now we disable it.
	e.EnableAutoSave(false)

	// Because AutoSave is disabled, the policy change only affects the policy in Casbin enforcer,
	// it doesn't affect the policy in the storage.
	_, err := e.AddPolicy("alice", "data1", "write")
	require.NoError(t, err)
	// Reload the policy from the storage to see the effect.
	err = e.LoadPolicy()
	require.NoError(t, err)
	// This is still the original policy.
	assertPolicy(t,
		[][]string{{"alice", "data1", "read"}, {"bob", "data2", "write"}, {"data2_admin", "data2", "read"}, {"data2_admin", "data2", "write"}},
		e.GetPolicy(),
	)

	// Now we enable the AutoSave.
	e.EnableAutoSave(true)

	// Because AutoSave is enabled, the policy change not only affects the policy in Casbin enforcer,
	// but also affects the policy in the storage.
	_, err = e.AddPolicy("alice", "data1", "write")
	require.NoError(t, err)
	// Reload the policy from the storage to see the effect.
	err = e.LoadPolicy()
	require.NoError(t, err)
	// The policy has a new rule: {"alice", "data1", "write"}.
	assertPolicy(t,
		[][]string{{"alice", "data1", "read"}, {"bob", "data2", "write"}, {"data2_admin", "data2", "read"}, {"data2_admin", "data2", "write"}, {"alice", "data1", "write"}},
		e.GetPolicy(),
	)

	// Aditional AddPolicy have no effect
	_, err = e.AddPolicy("alice", "data1", "write")
	require.NoError(t, err)
	err = e.LoadPolicy()
	require.NoError(t, err)
	assertPolicy(t,
		[][]string{{"alice", "data1", "read"}, {"bob", "data2", "write"}, {"data2_admin", "data2", "read"}, {"data2_admin", "data2", "write"}, {"alice", "data1", "write"}},
		e.GetPolicy(),
	)

	_, err = e.AddPolicies([][]string{
		{"bob", "data2", "read"},
		{"alice", "data2", "write"},
		{"alice", "data2", "read"},
		{"bob", "data1", "write"},
		{"bob", "data1", "read"},
	})
	require.NoError(t, err)
	// Reload the policy from the storage to see the effect.
	err = e.LoadPolicy()
	require.NoError(t, err)
	// The policy has a new rule: {"alice", "data1", "write"}.
	assertPolicy(t,
		[][]string{
			{"alice", "data1", "read"},
			{"bob", "data2", "write"},
			{"data2_admin", "data2", "read"},
			{"data2_admin", "data2", "write"},
			{"alice", "data1", "write"},
			{"bob", "data2", "read"},
			{"alice", "data2", "write"},
			{"alice", "data2", "read"},
			{"bob", "data1", "write"},
			{"bob", "data1", "read"},
		},
		e.GetPolicy(),
	)

	require.NoError(t, err)
}

func testConstructorOptions(t *testing.T, a *Adapter, e *casbin.Enforcer) {
	connStr := os.Getenv("PG_CONN")
	require.NotEmpty(t, connStr, "must run with non-empty PG_CONN")
	opts, err := pgx.ParseConfig(connStr)
	require.NoError(t, err)

	a, err = NewAdapter(opts, WithDatabase("test_pgxadapter"))
	require.NoError(t, err)
	defer a.Close()

	e, err = casbin.NewEnforcer("testdata/rbac_model.conf", a)
	require.NoError(t, err)

	assertPolicy(t,
		[][]string{{"alice", "data1", "read"}, {"bob", "data2", "write"}, {"data2_admin", "data2", "read"}, {"data2_admin", "data2", "write"}},
		e.GetPolicy(),
	)
}

func testRemovePolicy(t *testing.T, a *Adapter, e *casbin.Enforcer) {
	_, err := e.RemovePolicy("alice", "data1", "read")
	require.NoError(t, err)

	assertPolicy(t,
		[][]string{{"bob", "data2", "write"}, {"data2_admin", "data2", "read"}, {"data2_admin", "data2", "write"}},
		e.GetPolicy(),
	)

	err = e.LoadPolicy()
	require.NoError(t, err)

	assertPolicy(t,
		[][]string{{"bob", "data2", "write"}, {"data2_admin", "data2", "read"}, {"data2_admin", "data2", "write"}},
		e.GetPolicy(),
	)

	_, err = e.RemovePolicies([][]string{
		{"data2_admin", "data2", "read"},
		{"data2_admin", "data2", "write"},
	})
	require.NoError(t, err)

	assertPolicy(t,
		[][]string{{"bob", "data2", "write"}},
		e.GetPolicy(),
	)
}

func testRemoveFilteredPolicy(t *testing.T, a *Adapter, e *casbin.Enforcer) {
	_, err := e.RemoveFilteredPolicy(0, "", "data2")
	require.NoError(t, err)

	assertPolicy(t,
		[][]string{{"alice", "data1", "read"}},
		e.GetPolicy(),
	)

	err = e.LoadPolicy()
	require.NoError(t, err)

	assertPolicy(t,
		[][]string{{"alice", "data1", "read"}},
		e.GetPolicy(),
	)
}

func testLoadFilteredPolicy(t *testing.T, a *Adapter, e *casbin.Enforcer) {
	e, err := casbin.NewEnforcer("testdata/rbac_model.conf", a)
	require.NoError(t, err)

	err = e.LoadFilteredPolicy(&Filter{
		P: []string{"", "", "read"},
	})
	require.NoError(t, err)
	assert.True(t, e.IsFiltered())
	assertPolicy(t,
		[][]string{{"alice", "data1", "read"}, {"data2_admin", "data2", "read"}},
		e.GetPolicy(),
	)
}

func testLoadFilteredGroupingPolicy(t *testing.T, a *Adapter, e *casbin.Enforcer) {
	e, err := casbin.NewEnforcer("testdata/rbac_model.conf", a)
	require.NoError(t, err)

	err = e.LoadFilteredPolicy(&Filter{
		G: []string{"bob"},
	})
	require.NoError(t, err)
	assert.True(t, e.IsFiltered())
	assertPolicy(t, [][]string{}, e.GetGroupingPolicy())

	e, err = casbin.NewEnforcer("testdata/rbac_model.conf", a)
	require.NoError(t, err)

	err = e.LoadFilteredPolicy(&Filter{
		G: []string{"alice"},
	})
	require.NoError(t, err)
	assert.True(t, e.IsFiltered())
	assertPolicy(t, [][]string{{"alice", "data2_admin"}}, e.GetGroupingPolicy())
}

func testLoadFilteredPolicyNilFilter(t *testing.T, a *Adapter, e *casbin.Enforcer) {
	e, err := casbin.NewEnforcer("testdata/rbac_model.conf", a)
	require.NoError(t, err)

	err = e.LoadFilteredPolicy(nil)
	require.NoError(t, err)

	assert.False(t, e.IsFiltered())
	assertPolicy(t,
		[][]string{{"alice", "data1", "read"}, {"bob", "data2", "write"}, {"data2_admin", "data2", "read"}, {"data2_admin", "data2", "write"}},
		e.GetPolicy(),
	)
}

func testSavePolicyClearPreviousData(t *testing.T, a *Adapter, e *casbin.Enforcer) {
	e.EnableAutoSave(false)
	policies := e.GetPolicy()
	// clone slice to avoid shufling elements
	policies = append(policies[:0:0], policies...)
	for _, p := range policies {
		_, err := e.RemovePolicy(p)
		require.NoError(t, err)
	}
	policies = e.GetGroupingPolicy()
	policies = append(policies[:0:0], policies...)
	for _, p := range policies {
		_, err := e.RemoveGroupingPolicy(p)
		require.NoError(t, err)
	}
	assertPolicy(t,
		[][]string{},
		e.GetPolicy(),
	)

	err := e.SavePolicy()
	require.NoError(t, err)

	err = e.LoadPolicy()
	require.NoError(t, err)
	assertPolicy(t,
		[][]string{},
		e.GetPolicy(),
	)
}

func testUpdatePolicy(t *testing.T, a *Adapter, e *casbin.Enforcer) {
	var err error
	e, err = casbin.NewEnforcer("testdata/rbac_model.conf", "testdata/rbac_policy.csv")
	require.NoError(t, err)

	e.SetAdapter(a)

	err = e.SavePolicy()
	require.NoError(t, err)

	_, err = e.UpdatePolicies([][]string{{"alice", "data1", "read"}, {"bob", "data2", "write"}}, [][]string{{"bob", "data1", "read"}, {"alice", "data2", "write"}})
	require.NoError(t, err)

	err = e.LoadPolicy()
	require.NoError(t, err)

	assertPolicy(t, e.GetPolicy(), [][]string{{"data2_admin", "data2", "read"}, {"data2_admin", "data2", "write"}, {"bob", "data1", "read"}, {"alice", "data2", "write"}})

	_, err = e.UpdatePolicy([]string{"bob", "data1", "read"}, []string{"alice", "data1", "read"})
	require.NoError(t, err)

	assertPolicy(t, e.GetPolicy(), [][]string{{"data2_admin", "data2", "read"}, {"data2_admin", "data2", "write"}, {"alice", "data1", "read"}, {"alice", "data2", "write"}})
}

func testUpdatePolicyWithLoadFilteredPolicy(t *testing.T, a *Adapter, e *casbin.Enforcer) {
	var err error
	e, err = casbin.NewEnforcer("testdata/rbac_model.conf", "testdata/rbac_policy.csv")
	require.NoError(t, err)

	e.SetAdapter(a)

	err = e.SavePolicy()
	require.NoError(t, err)

	err = e.LoadFilteredPolicy(&Filter{P: []string{"data2_admin"}})
	require.NoError(t, err)

	_, err = e.UpdatePolicies(e.GetPolicy(), [][]string{{"bob", "data2", "read"}, {"alice", "data2", "write"}})
	require.NoError(t, err)

	err = e.LoadPolicy()
	require.NoError(t, err)

	assertPolicy(t, e.GetPolicy(), [][]string{{"alice", "data1", "read"}, {"bob", "data2", "write"}, {"bob", "data2", "read"}, {"alice", "data2", "write"}})
}

func TestAdapter(t *testing.T) {
	connStr := os.Getenv("PG_CONN")
	require.NotEmpty(t, connStr, "must run with non-empty PG_CONN")
	defer dropDB(t, "test_pgxadapter")
	a, err := NewAdapter(connStr, WithDatabase("test_pgxadapter"))
	require.NoError(t, err)
	defer a.Close()

	e, err := casbin.NewEnforcer("testdata/rbac_model.conf", "testdata/rbac_policy.csv")
	require.NoError(t, err)

	type subtest struct {
		Name string
		F    func(t *testing.T, a *Adapter, e *casbin.Enforcer)
	}

	t.Run("", func(t *testing.T) {
		for _, st := range []subtest{
			{"SaveLoad", testSaveLoad},
			{"AutoSave", testAutoSave},
			{"RemovePolicy", testRemovePolicy},
			{"RemoveFilteredPolicy", testRemoveFilteredPolicy},
			{"LoadFilteredPolicy", testLoadFilteredPolicy},
			{"LoadFilteredGroupingPolicy", testLoadFilteredGroupingPolicy},
			{"LoadFilteredPolicyNilFilter", testLoadFilteredPolicyNilFilter},
			{"SavePolicyClearPreviousData", testSavePolicyClearPreviousData},
			{"UpdatePolicy", testUpdatePolicy},
			{"UpdatePolicyWithLoadFilteredPolicy", testUpdatePolicyWithLoadFilteredPolicy},
			{"ConstructorOptions", testConstructorOptions},
		} {
			st := st
			t.Run(st.Name, func(t *testing.T) {
				// This is a trick to save the current policy to the DB.
				// We can't call e.SavePolicy() because the adapter in the enforcer is still the file adapter.
				// The current policy means the policy in the Casbin enforcer (aka in memory).
				err = a.SavePolicy(e.GetModel())
				require.NoError(t, err)
				e2, err := casbin.NewEnforcer("testdata/rbac_model.conf", a)
				require.NoError(t, err)
				st.F(t, a, e2)
			})
		}
	})
}
