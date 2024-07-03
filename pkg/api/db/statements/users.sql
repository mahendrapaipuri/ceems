INSERT INTO users (uid,cluster_id,resource_manager,name,projects,tags,last_updated_at) VALUES (:uid,:cluster_id,:resource_manager,:name,:projects,:tags,:last_updated_at) ON CONFLICT(cluster_id,name) DO UPDATE SET
  uid = :uid,
  cluster_id = :cluster_id,
  resource_manager = :resource_manager,
  name = :name,
  projects = :projects,
  tags = :tags,
  last_updated_at = :last_updated_at 
