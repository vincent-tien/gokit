package testutil

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ── Dialect tests ──

func TestDialectFromEnv_Default(t *testing.T) {
	t.Setenv("GOKIT_DIALECT", "")
	assert.Equal(t, Postgres, DialectFromEnv())
}

func TestDialectFromEnv_Postgres(t *testing.T) {
	t.Setenv("GOKIT_DIALECT", "postgres")
	assert.Equal(t, Postgres, DialectFromEnv())
}

func TestDialectFromEnv_MySQL(t *testing.T) {
	t.Setenv("GOKIT_DIALECT", "mysql")
	assert.Equal(t, MySQL, DialectFromEnv())
}

func TestDialectFromEnv_MariaDB(t *testing.T) {
	t.Setenv("GOKIT_DIALECT", "mariadb")
	assert.Equal(t, MySQL, DialectFromEnv())
}

func TestDialectFromEnv_CaseInsensitive(t *testing.T) {
	t.Setenv("GOKIT_DIALECT", "MySQL")
	assert.Equal(t, MySQL, DialectFromEnv())
}

func TestDialect_Driver(t *testing.T) {
	assert.Equal(t, "pgx", Postgres.Driver())
	assert.Equal(t, "mysql", MySQL.Driver())
}

func TestDialect_DefaultDSN(t *testing.T) {
	pgDSN := Postgres.DefaultDSN()
	assert.Contains(t, pgDSN, "postgres://")
	assert.Contains(t, pgDSN, "5432")

	myDSN := MySQL.DefaultDSN()
	assert.Contains(t, myDSN, "tcp(localhost:3306)")
	assert.Contains(t, myDSN, "parseTime=true")
}

func TestDialect_String(t *testing.T) {
	assert.Equal(t, "postgres", Postgres.String())
	assert.Equal(t, "mysql", MySQL.String())
}

// ── Fixture tests ──

func TestFixedTime_IsDeterministic(t *testing.T) {
	expected := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, expected, FixedTime)
	assert.Equal(t, FixedTime, FixedTime, "must be same on repeated access")
}

func TestFixedTimeAt_Ordering(t *testing.T) {
	t0 := FixedTime
	t1 := FixedTimeAt(1)
	t2 := FixedTimeAt(2)

	assert.True(t, t1.After(t0))
	assert.True(t, t2.After(t1))
	assert.Equal(t, time.Hour, t1.Sub(t0))
	assert.Equal(t, 2*time.Hour, t2.Sub(t0))
}

func TestFixedTimeAt_Negative(t *testing.T) {
	before := FixedTimeAt(-1)
	assert.True(t, before.Before(FixedTime))
}

func TestUUID_Deterministic(t *testing.T) {
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", UUID(1))
	assert.Equal(t, "00000000-0000-0000-0000-000000000042", UUID(42))
	assert.Equal(t, UUID(1), UUID(1), "same n must return same value")
}

func TestUUID_DifferentN(t *testing.T) {
	assert.NotEqual(t, UUID(1), UUID(2))
}

func TestRandomUUID_Unique(t *testing.T) {
	u1 := RandomUUID()
	u2 := RandomUUID()
	assert.NotEqual(t, u1, u2)
	assert.Len(t, u1, 36, "UUID format: 8-4-4-4-12")
}

// ── Assert tests ──

func TestJSONContains_MatchesKey(t *testing.T) {
	body := []byte(`{"name":"Alice","age":30}`)

	// Should not fail.
	JSONContains(t, body, "name", "Alice")
	JSONContains(t, body, "age", 30)
}

func TestJSONContains_MissingKey(t *testing.T) {
	body := []byte(`{"name":"Alice"}`)
	ft := &fakeTB{}
	JSONContains(ft, body, "missing", "value")
	assert.True(t, ft.failed, "should fail on missing key")
}

func TestJSONContains_WrongValue(t *testing.T) {
	body := []byte(`{"name":"Alice"}`)
	ft := &fakeTB{}
	JSONContains(ft, body, "name", "Bob")
	assert.True(t, ft.failed, "should fail on wrong value")
}

func TestJSONContains_InvalidJSON(t *testing.T) {
	ft := &fakeTB{}
	JSONContains(ft, []byte(`not json`), "key", "val")
	assert.True(t, ft.failed, "should fail on invalid JSON")
}

func TestJSONEqual_SameContent(t *testing.T) {
	a := []byte(`{"b":2,"a":1}`)
	b := []byte(`{"a":1,"b":2}`)
	JSONEqual(t, a, b)
}

func TestJSONEqual_DifferentContent(t *testing.T) {
	a := []byte(`{"a":1}`)
	b := []byte(`{"a":2}`)
	ft := &fakeTB{}
	JSONEqual(ft, a, b)
	assert.True(t, ft.failed)
}

func TestJSONEqual_InvalidExpected(t *testing.T) {
	ft := &fakeTB{}
	JSONEqual(ft, []byte(`bad`), []byte(`{"a":1}`))
	assert.True(t, ft.failed)
}

func TestJSONEqual_InvalidActual(t *testing.T) {
	ft := &fakeTB{}
	JSONEqual(ft, []byte(`{"a":1}`), []byte(`bad`))
	assert.True(t, ft.failed)
}

// ── DB connection tests (skip if no DATABASE_URL) ──

func TestDB_SkipWithoutDatabase(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	db := DB(t)
	assert.NotNil(t, db)
}

func TestPgPool_SkipWithoutDatabase(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	pool := PgPool(t)
	assert.NotNil(t, pool)
}

// fakeTB captures Fatalf calls without killing the test process.
type fakeTB struct {
	testing.TB
	failed bool
	msg    string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.failed = true
	_ = format
	_ = args
}
