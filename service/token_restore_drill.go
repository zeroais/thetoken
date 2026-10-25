package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// TOKEN: monthly restore drill (Cloud Run job token-prod-restore-drill,
// TOKEN_RESTORE_DRILL_ONLY=true; infra/terraform-gcp/restore_drill.tf).
//
//  1. Count the rows of the money, identity and configuration tables
//     (model.TokenDrillTables) at A, pick the restore time T just after,
//     count again at B.
//  2. Wait until the transaction log covering T is archived, then clone the
//     Cloud SQL instance to T (point-in-time recovery: the last backup plus
//     the log).
//  3. Connect to the clone over the private network and check every table:
//     its count must lie between the counts at A and B (rows only appear in
//     that window, so anything else is lost or extra data).
//  4. Delete the clone, always.
//
// Cloud SQL Admin API calls use the job's own service account (metadata
// server token). Environment: TOKEN_DRILL_PROJECT, TOKEN_DRILL_INSTANCE (the
// live instance), and SQL_DSN (the live database, as for the app).
const tokenSQLAdminURL = "https://sqladmin.googleapis.com/v1"

// tokenDrillLogWait is how long the drill waits after T before cloning, so
// the transaction log covering T has been archived.
var tokenDrillLogWait = 10 * time.Minute

func TokenRestoreDrill(ctx context.Context) error {
	project, instance := os.Getenv("TOKEN_DRILL_PROJECT"), os.Getenv("TOKEN_DRILL_INSTANCE")
	dsn := os.Getenv("SQL_DSN")
	if project == "" || instance == "" || !strings.HasPrefix(dsn, "postgres") {
		return errors.New("needs TOKEN_DRILL_PROJECT, TOKEN_DRILL_INSTANCE and a PostgreSQL SQL_DSN")
	}

	before, err := model.TokenTableCounts(model.DB)
	if err != nil {
		return err
	}
	restoreAt := time.Now().UTC().Add(time.Second).Truncate(time.Millisecond)
	time.Sleep(time.Until(restoreAt.Add(time.Second)))
	after, err := model.TokenTableCounts(model.DB)
	if err != nil {
		return err
	}
	common.SysLog(fmt.Sprintf("TOKEN restore drill: restore point %s; waiting %s for the transaction log", restoreAt.Format(time.RFC3339Nano), tokenDrillLogWait))
	time.Sleep(tokenDrillLogWait)

	clone := fmt.Sprintf("%s-drill-%s", instance, time.Now().UTC().Format("20060102-1504"))
	started := time.Now()
	err = tokenSQLAdminWait(ctx, project, http.MethodPost, fmt.Sprintf("/projects/%s/instances/%s/clone", project, instance),
		map[string]any{"cloneContext": map[string]any{"destinationInstanceName": clone, "pointInTime": restoreAt.Format(time.RFC3339Nano)}})
	defer func() {
		if derr := tokenDeleteDrillClone(context.Background(), project, clone); derr != nil {
			common.SysError(fmt.Sprintf("TOKEN restore drill: clone %s NOT deleted, delete it by hand: %v", clone, derr))
		} else {
			common.SysLog(fmt.Sprintf("TOKEN restore drill: clone %s deleted", clone))
		}
	}()
	if err != nil {
		return fmt.Errorf("clone to %s: %w", restoreAt.Format(time.RFC3339), err)
	}
	common.SysLog(fmt.Sprintf("TOKEN restore drill: clone %s ready after %s", clone, time.Since(started).Round(time.Second)))

	var inst struct {
		IPAddresses []tokenSQLAddress `json:"ipAddresses"`
	}
	if err := tokenSQLAdmin(ctx, http.MethodGet, fmt.Sprintf("/projects/%s/instances/%s", project, clone), nil, &inst); err != nil {
		return err
	}
	idx := slices.IndexFunc(inst.IPAddresses, func(a tokenSQLAddress) bool { return a.Type == "PRIVATE" })
	if idx < 0 {
		return fmt.Errorf("clone %s has no private address", clone)
	}
	cloneDSN, err := tokenDSNWithHost(dsn, inst.IPAddresses[idx].IPAddress)
	if err != nil {
		return err
	}
	db, closeDB, err := model.TokenOpenPostgres(cloneDSN)
	if err != nil {
		return fmt.Errorf("connect to the clone: %w", err)
	}
	defer closeDB()
	restored, err := model.TokenTableCounts(db)
	if err != nil {
		return fmt.Errorf("clone: %w", err)
	}
	return tokenCheckDrillCounts(before, after, restored)
}

type tokenSQLAddress struct {
	Type      string `json:"type"`
	IPAddress string `json:"ipAddress"`
}

// tokenCheckDrillCounts passes when every table of the clone has a count
// between its live counts just before and just after the restore point.
func tokenCheckDrillCounts(before, after, restored map[string]int64) error {
	var bad []string
	for _, table := range model.TokenDrillTables {
		lo, hi := min(before[table], after[table]), max(before[table], after[table])
		got := restored[table]
		mark := "ok"
		if got < lo || got > hi {
			mark = "MISMATCH"
			bad = append(bad, table)
		}
		common.SysLog(fmt.Sprintf("TOKEN restore drill: %-24s live %d..%d, restored %d %s", table, lo, hi, got, mark))
	}
	if len(bad) > 0 {
		return fmt.Errorf("restored counts differ for %s", strings.Join(bad, ", "))
	}
	return nil
}

func tokenDSNWithHost(dsn, host string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", errors.New("SQL_DSN is not a URL")
	}
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	u.Host = net.JoinHostPort(host, port)
	return u.String(), nil
}

func tokenDeleteDrillClone(ctx context.Context, project, clone string) error {
	path := fmt.Sprintf("/projects/%s/instances/%s", project, clone)
	if err := tokenSQLAdminWait(ctx, project, http.MethodDelete, path, nil); err == nil {
		return nil
	}
	// A clone may inherit deletion protection: turn it off, then delete.
	if err := tokenSQLAdminWait(ctx, project, http.MethodPatch, path, map[string]any{"settings": map[string]any{"deletionProtectionEnabled": false}}); err != nil {
		return err
	}
	return tokenSQLAdminWait(ctx, project, http.MethodDelete, path, nil)
}

// tokenSQLAdminWait sends a request that returns a Cloud SQL operation and
// waits until the operation is done (clones take tens of minutes).
func tokenSQLAdminWait(ctx context.Context, project, method, path string, body any) error {
	var op struct {
		Name string `json:"name"`
	}
	if err := tokenSQLAdmin(ctx, method, path, body, &op); err != nil {
		return err
	}
	deadline := time.Now().Add(50 * time.Minute)
	for time.Now().Before(deadline) {
		var status struct {
			Status string `json:"status"`
			Error  *struct {
				Errors []struct {
					Message string `json:"message"`
				} `json:"errors"`
			} `json:"error"`
		}
		if err := tokenSQLAdmin(ctx, http.MethodGet, fmt.Sprintf("/projects/%s/operations/%s", project, op.Name), nil, &status); err != nil {
			return err
		}
		if status.Status == "DONE" {
			if status.Error != nil && len(status.Error.Errors) > 0 {
				return errors.New(status.Error.Errors[0].Message)
			}
			return nil
		}
		time.Sleep(20 * time.Second)
	}
	return fmt.Errorf("operation %s not done after 50 minutes", op.Name)
}

func tokenSQLAdmin(ctx context.Context, method, path string, body, out any) error {
	token, err := tokenMetadataToken(ctx)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		data, err := common.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, tokenSQLAdminURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Cloud SQL Admin %s %s: HTTP %d: %.300s", method, path, resp.StatusCode, raw)
	}
	if out == nil {
		return nil
	}
	return common.Unmarshal(raw, out)
}

// tokenMetadataToken is the job's own service account token.
func tokenMetadataToken(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("metadata server: %w", err)
	}
	defer resp.Body.Close()
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := common.DecodeJson(resp.Body, &tok); err != nil || tok.AccessToken == "" {
		return "", errors.New("metadata server: no access token")
	}
	return tok.AccessToken, nil
}
