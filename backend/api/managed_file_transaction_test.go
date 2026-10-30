// managed_file_transaction_test.go verifies managed files survive ambiguous commits and are removed on definite insert failures.

package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type managedFileTestConnector struct {
	insertErr error
	commitErr error
}

func (c *managedFileTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &managedFileTestConn{config: c}, nil
}

func (c *managedFileTestConnector) Driver() driver.Driver { return managedFileTestDriver{} }

type managedFileTestDriver struct{}

func (managedFileTestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("managed-file test driver requires OpenDB connector")
}

type managedFileTestConn struct {
	config *managedFileTestConnector
}

func (c *managedFileTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepared statements are not supported by managed-file test driver")
}

func (c *managedFileTestConn) Close() error { return nil }

func (c *managedFileTestConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *managedFileTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &managedFileTestTx{config: c.config}, nil
}

func (c *managedFileTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "FROM portal.portal_auth_sessions"):
		now := time.Now()
		return &managedFileTestRows{
			columns: []string{"user_id", "email", "display_name", "created_at", "is_admin", "expires_at"},
			values:  []driver.Value{"00000000-0000-4000-8000-000000000001", "test@example.invalid", "Test User", now, false, now.Add(time.Hour)},
		}, nil
	case strings.Contains(query, "INSERT INTO portal.teaching_resources"):
		if c.config.insertErr != nil {
			return nil, c.config.insertErr
		}
		return &managedFileTestRows{
			columns: []string{"resource_id"},
			values:  []driver.Value{"00000000-0000-4000-8000-000000000002"},
		}, nil
	default:
		return nil, errors.New("unexpected query in managed-file test driver")
	}
}

func (c *managedFileTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO portal.uploaded_images") {
		if c.config.insertErr != nil {
			return nil, c.config.insertErr
		}
		return driver.RowsAffected(1), nil
	}
	return nil, errors.New("unexpected exec in managed-file test driver")
}

type managedFileTestTx struct {
	config *managedFileTestConnector
}

func (tx *managedFileTestTx) Commit() error   { return tx.config.commitErr }
func (tx *managedFileTestTx) Rollback() error { return nil }

type managedFileTestRows struct {
	columns []string
	values  []driver.Value
	read    bool
}

func (rows *managedFileTestRows) Columns() []string { return rows.columns }
func (rows *managedFileTestRows) Close() error      { return nil }
func (rows *managedFileTestRows) Next(dest []driver.Value) error {
	if rows.read {
		return io.EOF
	}
	rows.read = true
	copy(dest, rows.values)
	return nil
}

func openManagedFileTestDB(t *testing.T, config *managedFileTestConnector) *sql.DB {
	t.Helper()
	db := sql.OpenDB(config)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func managedFileTestRequest(api *portalAPI, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://portal.example"+path, strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: portalCookie, Value: "test-session-token"})
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	return response
}

func TestImageUploadPreservesFileWhenCommitOutcomeIsUnknown(t *testing.T) {
	config := &managedFileTestConnector{commitErr: errors.New("simulated connection loss during COMMIT")}
	db := openManagedFileTestDB(t, config)
	dir := t.TempDir()
	api := &portalAPI{db: db, uploadDir: dir, searchCache: newSearchResultCache()}
	imageBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 't', 'e', 's', 't'}
	body, err := json.Marshal(map[string]string{"mime": "image/png", "data": base64.StdEncoding.EncodeToString(imageBytes)})
	if err != nil {
		t.Fatal(err)
	}
	response := managedFileTestRequest(api, http.MethodPost, "/uploads", string(body))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("upload status = %d body=%s; want 503 for ambiguous commit", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("image file count after ambiguous commit = %d, want 1 preserved file", len(entries))
	}
	if info, err := os.Stat(filepath.Join(dir, entries[0].Name())); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("preserved image is not a regular file: info=%v err=%v", info, err)
	}
}

func TestImageUploadRemovesFileWhenRegistrationFails(t *testing.T) {
	config := &managedFileTestConnector{insertErr: errors.New("simulated image registration failure")}
	db := openManagedFileTestDB(t, config)
	dir := t.TempDir()
	api := &portalAPI{db: db, uploadDir: dir, searchCache: newSearchResultCache()}
	imageBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 't', 'e', 's', 't'}
	body, err := json.Marshal(map[string]string{"mime": "image/png", "data": base64.StdEncoding.EncodeToString(imageBytes)})
	if err != nil {
		t.Fatal(err)
	}
	response := managedFileTestRequest(api, http.MethodPost, "/uploads", string(body))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("upload status = %d body=%s; want 500 for failed registration", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("image file count after rolled-back registration = %d, want 0", len(entries))
	}
}

func TestResourceCreatePreservesFileWhenCommitOutcomeIsUnknown(t *testing.T) {
	config := &managedFileTestConnector{commitErr: errors.New("simulated connection loss during COMMIT")}
	db := openManagedFileTestDB(t, config)
	dir := t.TempDir()
	api := &portalAPI{db: db, resourceDir: dir, searchCache: newSearchResultCache()}
	fileBytes := []byte("%PDF-1.7\\nambiguous commit")
	body, err := json.Marshal(map[string]string{
		"title": "Ambiguous resource", "description": "", "type": "其他", "grade": "",
		"filename": "ambiguous.pdf", "data": base64.StdEncoding.EncodeToString(fileBytes),
	})
	if err != nil {
		t.Fatal(err)
	}
	response := managedFileTestRequest(api, http.MethodPost, "/resources", string(body))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("resource create status = %d body=%s; want 503 for ambiguous commit", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("resource file count after ambiguous commit = %d, want 1 preserved file", len(entries))
	}
	if info, err := os.Stat(filepath.Join(dir, entries[0].Name())); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("preserved resource is not a regular file: info=%v err=%v", info, err)
	}
}

func TestResourceCreateRemovesFileWhenInsertFails(t *testing.T) {
	config := &managedFileTestConnector{insertErr: errors.New("simulated resource insert failure")}
	db := openManagedFileTestDB(t, config)
	dir := t.TempDir()
	api := &portalAPI{db: db, resourceDir: dir, searchCache: newSearchResultCache()}
	fileBytes := []byte("%PDF-1.7\\ninsert failure")
	body, err := json.Marshal(map[string]string{
		"title": "Failed resource", "description": "", "type": "其他", "grade": "",
		"filename": "failed.pdf", "data": base64.StdEncoding.EncodeToString(fileBytes),
	})
	if err != nil {
		t.Fatal(err)
	}
	response := managedFileTestRequest(api, http.MethodPost, "/resources", string(body))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("resource create status = %d body=%s; want 500 for failed insert", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("resource file count after rolled-back insert = %d, want 0", len(entries))
	}
}
