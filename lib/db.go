package lib

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

type InputModel interface {
	BindIn() []any
}

type OutputModel[T any] interface {
	BindOut() []any
	*T
}
type Database struct {
	*sql.DB
	queryCache map[string]*sql.Stmt
}

func Init(driver string, source string) (d *Database, err error) {
	db, err := sql.Open(driver, source)

	if err == nil {
		return &Database{
			DB:         db,
			queryCache: make(map[string]*sql.Stmt, 8),
		}, nil
	}

	return
}

type NoInput struct{}

func (n NoInput) BindIn() []any {
	return nil
}

type Stmt[T InputModel] struct {
	*sql.Stmt
	key string
	db  Database
}

func AllFields[T InputModel]() (ret []string) {
	var input T

	for _, p := range input.BindIn() {
		ret = append(ret, p.(sql.NamedArg).Name)
	}

	return ret
}

func Sql[T InputModel](db Database, ctx context.Context, query string) (ret Stmt[T], err error) {
	cachedQuery, ok := db.queryCache[query]

	if !ok {
		newQuery := query

		var t T
		fields := t.BindIn()
		if len(fields) > 0 {
			if _, named := fields[0].(sql.NamedArg); named {
				for i, v := range fields {
					namedArg := v.(sql.NamedArg)
					newQuery = strings.ReplaceAll(newQuery, ":"+namedArg.Name, "$"+strconv.Itoa(i+1))
				}
			}
		}

		cachedQuery, err = db.PrepareContext(ctx, newQuery)

		if err != nil {
			return
		}

		db.queryCache[query] = cachedQuery
	}

	ret.Stmt = cachedQuery
	ret.key = query
	ret.db = db
	return
}

func SqlEx[T InputModel](db Database, ctx context.Context, cacheKey string, queryGen func(...any) string, data ...any) (ret Stmt[T], err error) {
	cachedQuery, ok := db.queryCache[cacheKey]

	if !ok {
		newQuery := queryGen(data...)

		var t T
		fields := t.BindIn()
		if len(fields) > 0 {
			if _, named := fields[0].(sql.NamedArg); named {
				for i, v := range fields {
					namedArg := v.(sql.NamedArg)
					newQuery = strings.ReplaceAll(newQuery, ":"+namedArg.Name, "$"+strconv.Itoa(i+1))
				}
			}
		}

		cachedQuery, err = db.PrepareContext(ctx, newQuery)

		if err != nil {
			return
		}

		db.queryCache[cacheKey] = cachedQuery
	}

	ret.Stmt = cachedQuery
	ret.key = cacheKey
	ret.db = db
	return
}

func (s *Stmt[T]) Close() {
	delete(s.db.queryCache, s.key)
	s.Stmt.Close()
}

func (s *Stmt[T]) Exec(ctx context.Context, tx *sql.Tx, model T) (sql.Result, error) {
	args := model.BindIn()

	if len(args) > 0 {
		if _, named := args[0].(sql.NamedArg); named {
			for i, v := range args {
				args[i] = v.(sql.NamedArg).Value
			}
		}
	}

	if tx != nil {
		return tx.Stmt(s.Stmt).ExecContext(ctx, args...)
	}

	return s.ExecContext(ctx, args...)
}

func (s *Stmt[T]) Query(ctx context.Context, tx *sql.Tx, model T) (ret *sql.Rows, err error) {
	args := model.BindIn()

	if len(args) > 0 {
		if _, named := args[0].(sql.NamedArg); named {
			for i, v := range args {
				args[i] = v.(sql.NamedArg).Value
			}
		}
	}

	if tx != nil {
		return tx.Stmt(s.Stmt).QueryContext(ctx, args...)
	}

	return s.QueryContext(ctx, args...)
}

type Rows[T any, TModel OutputModel[T]] struct {
	*sql.Rows
}

func IterRows[T any, TModel OutputModel[T]](rows *sql.Rows) Rows[T, TModel] {
	return Rows[T, TModel]{rows}
}

func (r *Rows[T, TModel]) Next(ref ...*T) (ret TModel, ok bool) {
	if len(ref) == 0 {
		ret = new(T)
	} else {
		ret = ref[0]
	}
	ok = r.Rows.Next()

	if !ok {
		return
	}

	r.Rows.Scan(ret.BindOut()...)
	return
}
