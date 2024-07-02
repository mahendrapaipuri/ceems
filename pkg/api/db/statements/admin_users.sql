INSERT INTO admin_users (source,users,last_updated_at) VALUES (:source,:users,:last_updated_at) ON CONFLICT(source) DO UPDATE SET
  source = :source,
  users = :users,
  last_updated_at = :last_updated_at
