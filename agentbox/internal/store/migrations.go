package store

const schema = `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS projects (
	project_id TEXT PRIMARY KEY,
	enrollment_hash TEXT NOT NULL,
	source_policy TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS credential_profiles (
	id TEXT PRIMARY KEY,
	max_active_runs INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS credential_locks (
	profile_id TEXT PRIMARY KEY REFERENCES credential_profiles(id),
	active_operation_id TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS operations (
	operation_id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL REFERENCES projects(project_id),
	type TEXT NOT NULL,
	request_digest TEXT NOT NULL,
	state TEXT NOT NULL,
	result TEXT
);

CREATE TABLE IF NOT EXISTS receipts (
	project_id TEXT PRIMARY KEY REFERENCES projects(project_id),
	revision INTEGER NOT NULL,
	state TEXT NOT NULL,
	generation TEXT
);

CREATE TABLE IF NOT EXISTS generations (
	project_id TEXT NOT NULL REFERENCES projects(project_id),
	generation TEXT NOT NULL,
	PRIMARY KEY (project_id, generation)
);

CREATE TABLE IF NOT EXISTS source_leases (
	project_id TEXT PRIMARY KEY REFERENCES projects(project_id),
	generation TEXT,
	state TEXT NOT NULL,
	operation_id TEXT
);
CREATE TABLE IF NOT EXISTS runs (
	run_id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL REFERENCES projects(project_id),
	unit TEXT NOT NULL,
	state TEXT NOT NULL
);


CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	at TEXT NOT NULL,
	actor TEXT NOT NULL,
	old_state TEXT,
	new_state TEXT,
	result TEXT
);

CREATE TABLE IF NOT EXISTS backups (
	path TEXT PRIMARY KEY,
	created_at TEXT NOT NULL,
	verified INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS control_gate (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	writers_blocked INTEGER NOT NULL
);

INSERT OR IGNORE INTO control_gate (id, writers_blocked) VALUES (1, 0);
`
