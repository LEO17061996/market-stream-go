package store

// Thời gian lưu dạng BIGINT mili giây để cùng một câu SQL chạy được trên cả MySQL và SQLite.

var mysqlSchema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		username VARCHAR(50) NOT NULL,
		password_hash VARCHAR(100) NOT NULL,
		full_name VARCHAR(100) NOT NULL,
		created_at BIGINT NOT NULL,
		UNIQUE KEY uq_users_username (username)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS accounts (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		user_id BIGINT NOT NULL,
		code VARCHAR(20) NOT NULL,
		balance_vnd BIGINT NOT NULL,
		updated_at BIGINT NOT NULL,
		UNIQUE KEY uq_accounts_user (user_id),
		UNIQUE KEY uq_accounts_code (code),
		CONSTRAINT fk_accounts_user FOREIGN KEY (user_id) REFERENCES users (id),
		CONSTRAINT chk_accounts_balance CHECK (balance_vnd >= 0)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS margin_transactions (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		account_id BIGINT NOT NULL,
		kind VARCHAR(10) NOT NULL,
		amount_vnd BIGINT NOT NULL,
		status VARCHAR(10) NOT NULL,
		note VARCHAR(200) NOT NULL DEFAULT '',
		created_at BIGINT NOT NULL,
		KEY idx_tx_account_id (account_id, id),
		KEY idx_tx_pending (account_id, kind, status),
		CONSTRAINT fk_tx_account FOREIGN KEY (account_id) REFERENCES accounts (id),
		CONSTRAINT chk_tx_amount CHECK (amount_vnd > 0),
		CONSTRAINT chk_tx_kind CHECK (kind IN ('deposit', 'withdraw')),
		CONSTRAINT chk_tx_status CHECK (status IN ('pending', 'completed', 'rejected'))
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS audit_logs (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		user_id BIGINT NULL,
		action VARCHAR(50) NOT NULL,
		ip VARCHAR(64) NOT NULL,
		detail VARCHAR(255) NOT NULL DEFAULT '',
		created_at BIGINT NOT NULL,
		KEY idx_audit_user_time (user_id, created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
}

var sqliteSchema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		full_name TEXT NOT NULL,
		created_at INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL UNIQUE REFERENCES users (id),
		code TEXT NOT NULL UNIQUE,
		balance_vnd INTEGER NOT NULL CHECK (balance_vnd >= 0),
		updated_at INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS margin_transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL REFERENCES accounts (id),
		kind TEXT NOT NULL CHECK (kind IN ('deposit', 'withdraw')),
		amount_vnd INTEGER NOT NULL CHECK (amount_vnd > 0),
		status TEXT NOT NULL CHECK (status IN ('pending', 'completed', 'rejected')),
		note TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tx_account_id ON margin_transactions (account_id, id)`,
	`CREATE INDEX IF NOT EXISTS idx_tx_pending ON margin_transactions (account_id, kind, status)`,
	`CREATE TABLE IF NOT EXISTS audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NULL,
		action TEXT NOT NULL,
		ip TEXT NOT NULL,
		detail TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_audit_user_time ON audit_logs (user_id, created_at)`,
}
