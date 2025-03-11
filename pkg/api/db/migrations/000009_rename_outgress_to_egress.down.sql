ALTER TABLE daily_usage RENAME COLUMN total_egress_stats TO total_outgress_stats;
ALTER TABLE usage RENAME COLUMN total_egress_stats TO total_outgress_stats;
ALTER TABLE units RENAME COLUMN total_egress_stats TO total_outgress_stats;
