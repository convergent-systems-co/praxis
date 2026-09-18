package lifecycle

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// InvocationRuntimeBindingsTable is the exact table this ADR-088 §2
// storage_schema transition structurally corrects.
const InvocationRuntimeBindingsTable = "invocation_runtime_bindings"

const invocationRuntimeBindingsIndex = "idx_invocation_runtime_bindings_package"

// invocationRuntimeBindingsColumn is the live-introspected shape of one
// column, from PRAGMA table_info. Column order is preserved (it is part
// of the observed structural identity), never re-sorted.
type invocationRuntimeBindingsColumn struct {
	Name         string
	Type         string
	NotNull      bool
	HasDefault   bool
	DefaultValue string
	PrimaryKey   int // 0 = not part of the PK; otherwise 1-based PK column order
	Hidden       int // table_xinfo: generated/hidden-column classification
}

// invocationRuntimeBindingsForeignKey is the live-introspected shape of
// one foreign key, from PRAGMA foreign_key_list grouped by id. From/To are
// ordered by the FK's own column sequence (PRAGMA foreign_key_list's seq),
// which for a composite key encodes which local column maps to which
// referenced column — order matters and is never re-sorted.
type invocationRuntimeBindingsForeignKey struct {
	Table    string
	From     []string
	To       []string
	OnDelete string
	OnUpdate string
}

// invocationRuntimeBindingsIndexShape is the live-introspected shape of
// one index, from PRAGMA index_list + PRAGMA index_info.
type invocationRuntimeBindingsIndexShape struct {
	Name       string
	Unique     bool
	Origin     string
	Partial    bool
	Columns    []string
	Definition string
}

// InvocationRuntimeBindingsShape is the complete live structural
// description of invocation_runtime_bindings this transition introspects
// and hashes — never assumed from schema_meta or migration filenames
// (ADR-088 §2's central evidence requirement).
type InvocationRuntimeBindingsShape struct {
	TableDefinition string
	Columns         []invocationRuntimeBindingsColumn
	ForeignKeys     []invocationRuntimeBindingsForeignKey
	Indexes         []invocationRuntimeBindingsIndexShape
	Triggers        []string
}

// sqlQueryer is satisfied by both *sql.DB and *sql.Tx, letting
// introspection serve both a pre-transaction Preflight check and an
// in-transaction Apply re-check from one implementation.
type sqlQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// IntrospectInvocationRuntimeBindingsShape reads the complete live
// structural shape of invocation_runtime_bindings — columns, primary key
// order, foreign keys (including ON DELETE/ON UPDATE actions), and
// indexes — directly from SQLite's own schema catalog. It never reads
// schema_meta.value or trusts migration status as evidence of actual
// shape (ADR-088 §2): the exact defect this recovery exists to fix is
// that schema_meta can claim a version whose live shape does not match
// what current source expects. q may be a *sql.DB (Preflight, before any
// transaction) or a *sql.Tx (Apply's in-transaction predicate
// revalidation, ADR-088 §13.4) — the same query path serves both.
func IntrospectInvocationRuntimeBindingsShape(ctx context.Context, q sqlQueryer) (InvocationRuntimeBindingsShape, error) {
	tableDefinition, err := introspectSchemaDefinition(ctx, q, "table", InvocationRuntimeBindingsTable)
	if err != nil {
		return InvocationRuntimeBindingsShape{}, fmt.Errorf("introspect table definition: %w", err)
	}
	columns, err := introspectColumns(ctx, q, InvocationRuntimeBindingsTable)
	if err != nil {
		return InvocationRuntimeBindingsShape{}, fmt.Errorf("introspect columns: %w", err)
	}
	if len(columns) == 0 {
		return InvocationRuntimeBindingsShape{}, fmt.Errorf("table %q does not exist", InvocationRuntimeBindingsTable)
	}
	foreignKeys, err := introspectForeignKeys(ctx, q, InvocationRuntimeBindingsTable)
	if err != nil {
		return InvocationRuntimeBindingsShape{}, fmt.Errorf("introspect foreign keys: %w", err)
	}
	indexes, err := introspectIndexes(ctx, q, InvocationRuntimeBindingsTable)
	if err != nil {
		return InvocationRuntimeBindingsShape{}, fmt.Errorf("introspect indexes: %w", err)
	}
	triggers, err := introspectTriggers(ctx, q, InvocationRuntimeBindingsTable)
	if err != nil {
		return InvocationRuntimeBindingsShape{}, fmt.Errorf("introspect triggers: %w", err)
	}
	return InvocationRuntimeBindingsShape{TableDefinition: tableDefinition, Columns: columns, ForeignKeys: foreignKeys, Indexes: indexes, Triggers: triggers}, nil
}

func normalizeSQLiteDDL(definition string) string {
	definition = strings.NewReplacer("\"", "", "`", "", "[", "", "]", "", "(", " ( ", ")", " ) ", ",", " , ", ";", " ").Replace(definition)
	return strings.ToLower(strings.Join(strings.Fields(definition), " "))
}

func introspectSchemaDefinition(ctx context.Context, q sqlQueryer, kind, name string) (string, error) {
	rows, err := q.QueryContext(ctx, `SELECT COALESCE(sql,'') FROM sqlite_master WHERE type=? AND name=?`, kind, name)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", nil
	}
	var definition string
	if err := rows.Scan(&definition); err != nil {
		return "", err
	}
	return normalizeSQLiteDDL(definition), rows.Err()
}

func introspectTriggers(ctx context.Context, q sqlQueryer, table string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT COALESCE(sql,'') FROM sqlite_master WHERE type='trigger' AND tbl_name=? ORDER BY name`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var definition string
		if err := rows.Scan(&definition); err != nil {
			return nil, err
		}
		out = append(out, normalizeSQLiteDDL(definition))
	}
	return out, rows.Err()
}

func introspectColumns(ctx context.Context, q sqlQueryer, table string) ([]invocationRuntimeBindingsColumn, error) {
	rows, err := q.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_xinfo(%s)`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []invocationRuntimeBindingsColumn
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk, hidden int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk, &hidden); err != nil {
			return nil, err
		}
		out = append(out, invocationRuntimeBindingsColumn{Name: name, Type: ctype, NotNull: notNull != 0, HasDefault: dflt.Valid, DefaultValue: dflt.String, PrimaryKey: pk, Hidden: hidden})
	}
	return out, rows.Err()
}

func introspectForeignKeys(ctx context.Context, q sqlQueryer, table string) ([]invocationRuntimeBindingsForeignKey, error) {
	rows, err := q.QueryContext(ctx, fmt.Sprintf(`PRAGMA foreign_key_list(%s)`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type rawRow struct {
		id, seq               int
		refTable, from, to    string
		onUpdate, onDelete, m string
	}
	var raw []rawRow
	for rows.Next() {
		var r rawRow
		if err := rows.Scan(&r.id, &r.seq, &r.refTable, &r.from, &r.to, &r.onUpdate, &r.onDelete, &r.m); err != nil {
			return nil, err
		}
		raw = append(raw, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	byID := map[int]*invocationRuntimeBindingsForeignKey{}
	var order []int
	for _, r := range raw {
		fk, ok := byID[r.id]
		if !ok {
			fk = &invocationRuntimeBindingsForeignKey{Table: r.refTable, OnDelete: r.onDelete, OnUpdate: r.onUpdate}
			byID[r.id] = fk
			order = append(order, r.id)
		}
		fk.From = append(fk.From, r.from)
		fk.To = append(fk.To, r.to)
	}
	out := make([]invocationRuntimeBindingsForeignKey, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	// Multiple independent foreign keys (not the case for this table
	// today, but introspection must not assume a single-FK shape) are
	// ordered deterministically by referenced table then joined column
	// list, since PRAGMA foreign_key_list's own id ordering is not part
	// of the schema's observable identity.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Table != out[j].Table {
			return out[i].Table < out[j].Table
		}
		return fmt.Sprint(out[i].From) < fmt.Sprint(out[j].From)
	})
	return out, nil
}

func introspectIndexes(ctx context.Context, q sqlQueryer, table string) ([]invocationRuntimeBindingsIndexShape, error) {
	rows, err := q.QueryContext(ctx, fmt.Sprintf(`PRAGMA index_list(%s)`, table))
	if err != nil {
		return nil, err
	}
	type rawIndex struct {
		name, origin    string
		unique, partial bool
	}
	var raw []rawIndex
	for rows.Next() {
		var seq int
		var name, origin string
		var unique, partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			rows.Close()
			return nil, err
		}
		// Auto-indexes backing the PRIMARY KEY/UNIQUE constraints are
		// already fully captured by column PrimaryKey order; including
		// them again here would double-count the same structural fact
		// under a SQLite-internal, non-portable name.
		if origin == "pk" {
			continue
		}
		raw = append(raw, rawIndex{name: name, origin: origin, unique: unique != 0, partial: partial != 0})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	out := make([]invocationRuntimeBindingsIndexShape, 0, len(raw))
	for _, idx := range raw {
		cols, err := introspectIndexColumns(ctx, q, idx.name)
		if err != nil {
			return nil, err
		}
		definition, err := introspectIndexDefinition(ctx, q, idx.name)
		if err != nil {
			return nil, err
		}
		out = append(out, invocationRuntimeBindingsIndexShape{Name: idx.name, Unique: idx.unique, Origin: idx.origin, Partial: idx.partial, Columns: cols, Definition: definition})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func introspectIndexDefinition(ctx context.Context, q sqlQueryer, indexName string) (string, error) {
	return introspectSchemaDefinition(ctx, q, "index", indexName)
}

func introspectIndexColumns(ctx context.Context, q sqlQueryer, indexName string) ([]string, error) {
	rows, err := q.QueryContext(ctx, fmt.Sprintf(`PRAGMA index_info(%s)`, indexName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var seqno, cid int
		var name sql.NullString
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			return nil, err
		}
		cols = append(cols, name.String)
	}
	return cols, rows.Err()
}

// Digest returns the canonical sha256 digest of this structural shape, in
// the same "sha256:<hex>" form contracts.ValidateSHA256Digest expects.
// This is the ONLY value a governed Apply may use as
// ApplyResult.ResultingManifestDigest for the storage_schema transition —
// it is always computed from a value this function itself introspected,
// never echoed from an intended/target digest (ADR-088 §3).
func (s InvocationRuntimeBindingsShape) Digest() (string, error) {
	payload, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// invocationRuntimeBindingsShapeKind classifies a live-introspected shape
// against the two shapes this ADR-088 §2 transition recognizes. A third,
// unrecognized shape is never silently coerced into either.
type invocationRuntimeBindingsShapeKind int

const (
	shapeUnrecognized invocationRuntimeBindingsShapeKind = iota
	shapeHistoricalHandler
	shapeCanonicalRuntime
)

// HistoricalInvocationRuntimeBindingsShape returns the exact structural
// shape of the historical dogfood-divergent body of
// migrations/sqlite/0012_invocation_runtime_bindings.sql (commit
// 0084352), reproduced from primary source, not paraphrased: handler_id/
// handler_version/handler_digest columns, executable_binding_json present
// (0013 applies identically to both bodies), and the FK to
// invocation_registry WITHOUT ON DELETE CASCADE.
func HistoricalInvocationRuntimeBindingsShape() InvocationRuntimeBindingsShape {
	return InvocationRuntimeBindingsShape{
		TableDefinition: normalizeSQLiteDDL(`CREATE TABLE invocation_runtime_bindings (entry_point_id TEXT NOT NULL, package_version TEXT NOT NULL, content_digest TEXT NOT NULL, package_id TEXT NOT NULL, contract_digest TEXT NOT NULL, handler_id TEXT NOT NULL, handler_version TEXT NOT NULL, handler_digest TEXT NOT NULL, registered_at TEXT NOT NULL, executable_binding_json BLOB, PRIMARY KEY (entry_point_id, package_version, content_digest), FOREIGN KEY (entry_point_id, package_version, content_digest) REFERENCES invocation_registry(entry_point_id, package_version, content_digest))`),
		Columns: []invocationRuntimeBindingsColumn{
			{Name: "entry_point_id", Type: "TEXT", NotNull: true, PrimaryKey: 1},
			{Name: "package_version", Type: "TEXT", NotNull: true, PrimaryKey: 2},
			{Name: "content_digest", Type: "TEXT", NotNull: true, PrimaryKey: 3},
			{Name: "package_id", Type: "TEXT", NotNull: true},
			{Name: "contract_digest", Type: "TEXT", NotNull: true},
			{Name: "handler_id", Type: "TEXT", NotNull: true},
			{Name: "handler_version", Type: "TEXT", NotNull: true},
			{Name: "handler_digest", Type: "TEXT", NotNull: true},
			{Name: "registered_at", Type: "TEXT", NotNull: true},
			{Name: "executable_binding_json", Type: "BLOB"},
		},
		ForeignKeys: []invocationRuntimeBindingsForeignKey{
			{Table: "invocation_registry", From: []string{"entry_point_id", "package_version", "content_digest"}, To: []string{"entry_point_id", "package_version", "content_digest"}, OnDelete: "NO ACTION", OnUpdate: "NO ACTION"},
		},
		Indexes: []invocationRuntimeBindingsIndexShape{
			{Name: invocationRuntimeBindingsIndex, Unique: false, Origin: "c", Columns: []string{"package_id", "content_digest"}, Definition: normalizeSQLiteDDL("CREATE INDEX idx_invocation_runtime_bindings_package ON invocation_runtime_bindings(package_id, content_digest)")},
		},
	}
}

// CanonicalInvocationRuntimeBindingsShape returns the exact structural
// shape current source requires, reproduced from primary source
// (migrations/sqlite/0012_invocation_runtime_bindings.sql, commit
// 2860381, plus 0013's executable_binding_json addition): runtime_id/
// runtime_version/runtime_digest columns, and the FK to
// invocation_registry WITH ON DELETE CASCADE.
func CanonicalInvocationRuntimeBindingsShape() InvocationRuntimeBindingsShape {
	return InvocationRuntimeBindingsShape{
		TableDefinition: normalizeSQLiteDDL(`CREATE TABLE invocation_runtime_bindings (entry_point_id TEXT NOT NULL, package_version TEXT NOT NULL, content_digest TEXT NOT NULL, package_id TEXT NOT NULL, contract_digest TEXT NOT NULL, runtime_id TEXT NOT NULL, runtime_version TEXT NOT NULL, runtime_digest TEXT NOT NULL, registered_at TEXT NOT NULL, executable_binding_json BLOB, PRIMARY KEY (entry_point_id, package_version, content_digest), FOREIGN KEY (entry_point_id, package_version, content_digest) REFERENCES invocation_registry(entry_point_id, package_version, content_digest) ON DELETE CASCADE)`),
		Columns: []invocationRuntimeBindingsColumn{
			{Name: "entry_point_id", Type: "TEXT", NotNull: true, PrimaryKey: 1},
			{Name: "package_version", Type: "TEXT", NotNull: true, PrimaryKey: 2},
			{Name: "content_digest", Type: "TEXT", NotNull: true, PrimaryKey: 3},
			{Name: "package_id", Type: "TEXT", NotNull: true},
			{Name: "contract_digest", Type: "TEXT", NotNull: true},
			{Name: "runtime_id", Type: "TEXT", NotNull: true},
			{Name: "runtime_version", Type: "TEXT", NotNull: true},
			{Name: "runtime_digest", Type: "TEXT", NotNull: true},
			{Name: "registered_at", Type: "TEXT", NotNull: true},
			{Name: "executable_binding_json", Type: "BLOB"},
		},
		ForeignKeys: []invocationRuntimeBindingsForeignKey{
			{Table: "invocation_registry", From: []string{"entry_point_id", "package_version", "content_digest"}, To: []string{"entry_point_id", "package_version", "content_digest"}, OnDelete: "CASCADE", OnUpdate: "NO ACTION"},
		},
		Indexes: []invocationRuntimeBindingsIndexShape{
			{Name: invocationRuntimeBindingsIndex, Unique: false, Origin: "c", Columns: []string{"package_id", "content_digest"}, Definition: normalizeSQLiteDDL("CREATE INDEX idx_invocation_runtime_bindings_package ON invocation_runtime_bindings(package_id, content_digest)")},
		},
	}
}

func classifyInvocationRuntimeBindingsShape(shape InvocationRuntimeBindingsShape) invocationRuntimeBindingsShapeKind {
	if shapesEqual(shape, HistoricalInvocationRuntimeBindingsShape()) {
		return shapeHistoricalHandler
	}
	if shapesEqual(shape, CanonicalInvocationRuntimeBindingsShape()) {
		return shapeCanonicalRuntime
	}
	return shapeUnrecognized
}

func shapesEqual(a, b InvocationRuntimeBindingsShape) bool {
	aj, aerr := json.Marshal(a)
	bj, berr := json.Marshal(b)
	if aerr != nil || berr != nil {
		return false
	}
	return string(aj) == string(bj)
}

// ErrUnrecognizedSchemaShape is returned when live introspection matches
// neither the historical nor the canonical invocation_runtime_bindings
// shape. It is never silently coerced into either recognized case.
var ErrUnrecognizedSchemaShape = errors.New("invocation_runtime_bindings live shape matches neither the historical nor canonical structural shape")
