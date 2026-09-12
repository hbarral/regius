DROP TABLE IF EXISTS tokens;


DROP TABLE IF EXISTS remember_tokens;


DROP TABLE IF EXISTS users;


CREATE TABLE users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  first_name TEXT NOT NULL,
  last_name TEXT NOT NULL,
  user_active INTEGER NOT NULL DEFAULT 0,
  email TEXT NOT NULL UNIQUE,
  password TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);


CREATE INDEX users_email_idx ON users (email);


CREATE TABLE remember_tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  remember_token TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE
);


CREATE INDEX remember_tokens_user_id_idx ON remember_tokens (user_id);


CREATE INDEX remember_tokens_remember_token_idx ON remember_tokens (remember_token);


CREATE TABLE tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  first_name TEXT NOT NULL,
  email TEXT NOT NULL,
  token TEXT NOT NULL,
  token_hash BLOB NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expiry DATETIME NOT NULL,
  FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE
);


CREATE INDEX tokens_user_id_idx ON tokens (user_id);


CREATE INDEX tokens_token_idx ON tokens (token);

