-- truncate tables to ensure idempotent loading
TRUNCATE TABLE users, user_roles RESTART IDENTITY CASCADE;

INSERT INTO users (id, email, name, created_at)
OVERRIDING SYSTEM VALUE
VALUES
  (1, 'alice@example.com', 'Alice Smith', '2024-01-15 10:00:00'),
  (2, 'bob@example.com', 'Bob Johnson', '2024-02-20 14:30:00'),
  (3, 'charlie@example.com', 'Charlie Brown', '2024-03-10 09:15:00');

INSERT INTO user_roles (user_id, role)
OVERRIDING SYSTEM VALUE
VALUES
  (1, 'admin'),
  (2, 'user'),
  (3, 'user');
