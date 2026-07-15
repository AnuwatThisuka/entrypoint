package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGO), registered as "sqlite"
)

// cacheDBName is the local storage buffer for asynchronous event processing.
// It lives under .git/ so it is never committed alongside project sources.
const cacheDBName = "entrypoint_cache.db"

// cacheSchema initializes the event buffer. Rows are appended by hooks and
// drained by the async processor, so an auto-increment id gives a stable
// FIFO order without depending on wall-clock timestamps.
const cacheSchema = `
CREATE TABLE IF NOT EXISTS event_buffer (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id   TEXT    NOT NULL,
    type       TEXT    NOT NULL,
    actor      TEXT,
    payload    TEXT,
    created_at TEXT    NOT NULL,
    processed  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_event_buffer_unprocessed
    ON event_buffer (processed, id);
`

func newInit() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize local entrypoint metadata (cache DB and Claude Code hooks) in this repository",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runInit(c.Context(), cwd)
		},
	}
}

// runInit performs the three setup steps in order: verify the repo, create the
// metadata dir + cache DB, and inject the standard hook configuration. Each
// step reports what it did so `init` is transparent when re-run.
func runInit(ctx context.Context, cwd string) error {
	gitDir := filepath.Join(cwd, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("no .git directory in %s — run this from the root of a git repository", cwd)
	}

	metaDir := filepath.Join(gitDir, "entrypoint")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return fmt.Errorf("create metadata directory: %w", err)
	}
	fmt.Printf("metadata directory ready: %s\n", metaDir)

	dbPath := filepath.Join(metaDir, cacheDBName)
	if err := initCacheDB(ctx, dbPath); err != nil {
		return fmt.Errorf("initialize cache database: %w", err)
	}
	fmt.Printf("event cache database ready: %s\n", dbPath)

	if err := injectHooks(cwd); err != nil {
		return fmt.Errorf("inject configuration hooks: %w", err)
	}

	fmt.Println("entrypoint initialized.")
	return nil
}

// initCacheDB opens (creating if absent) the SQLite buffer and applies the
// schema. The context bounds the schema write so init can't hang on a locked
// database file.
func initCacheDB(ctx context.Context, dbPath string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, cacheSchema); err != nil {
		return err
	}
	return nil
}

// standardHooks is the SessionEnd + PostToolUse wiring documented in the README:
// SessionEnd captures a packet, PostToolUse feeds the agent file-write log.
var standardHooks = map[string]any{
	"SessionEnd": []any{
		map[string]any{
			"hooks": []any{
				map[string]any{"type": "command", "command": "entrypoint hook"},
			},
		},
	},
	"PostToolUse": []any{
		map[string]any{
			"matcher": "Write|Edit|MultiEdit|NotebookEdit",
			"hooks": []any{
				map[string]any{"type": "command", "command": "entrypoint hook track"},
			},
		},
	},
}

// injectHooks writes the standard hook configuration into .claude/settings.json.
// An existing settings file with a "hooks" key is left untouched so we never
// clobber a user's customizations — init reports which case applied.
func injectHooks(cwd string) error {
	dir := filepath.Join(cwd, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "settings.json")

	settings := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("existing %s is not valid JSON: %w", path, err)
		}
		if _, ok := settings["hooks"]; ok {
			fmt.Printf("hooks already configured in %s — left unchanged.\n", path)
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	hooksRaw, err := json.Marshal(standardHooks)
	if err != nil {
		return err
	}
	settings["hooks"] = hooksRaw

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("configuration hooks injected: %s\n", path)
	return nil
}
