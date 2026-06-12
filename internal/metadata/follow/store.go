package follow

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) List(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT login FROM local_follows ORDER BY created_at DESC, login ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logins := make([]string, 0)
	for rows.Next() {
		var login string
		if err := rows.Scan(&login); err != nil {
			return nil, err
		}
		logins = append(logins, login)
	}
	return logins, rows.Err()
}

func (s *Store) Add(ctx context.Context, login string) error {
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return fmt.Errorf("empty login")
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO local_follows (login)
		VALUES ($1)
		ON CONFLICT (login) DO NOTHING
	`, login)
	return err
}

func (s *Store) Remove(ctx context.Context, login string) error {
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return fmt.Errorf("empty login")
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM local_follows WHERE login = $1`, login)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) Contains(ctx context.Context, login string) (bool, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM local_follows WHERE login = $1)`, login).Scan(&exists)
	return exists, err
}
