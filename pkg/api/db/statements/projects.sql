INSERT INTO projects (uid,cluster_id,resource_manager,name,users,tags,last_updated_at) VALUES (:uid,:cluster_id,:resource_manager,:name,:users,:tags,:last_updated_at) ON CONFLICT(cluster_id,name) DO UPDATE SET
  uid = :uid,
  cluster_id = :cluster_id,
  resource_manager = :resource_manager,
  name = :name,
  users = :users,
  tags = :tags,
  last_updated_at = :last_updated_at  
