ALTER TABLE units RENAME COLUMN total_outgress_stats TO total_egress_stats;
ALTER TABLE usage RENAME COLUMN total_outgress_stats TO total_egress_stats;
ALTER TABLE daily_usage RENAME COLUMN total_outgress_stats TO total_egress_stats;
