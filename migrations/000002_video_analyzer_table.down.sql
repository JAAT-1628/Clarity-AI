DROP TABLE IF EXISTS key_insights;
DROP TABLE IF EXISTS questions;
DROP TABLE IF EXISTS topics;
DROP TABLE IF EXISTS video_chunks;
DROP TABLE IF EXISTS video_analyses;

DROP INDEX IF EXISTS idx_key_insights_analysis_id;
DROP INDEX IF EXISTS idx_questions_analysis_id;
DROP INDEX IF EXISTS idx_topics_analysis_id;
DROP INDEX IF EXISTS idx_video_chunks_status;
DROP INDEX IF EXISTS idx_video_chunks_analysis_id;
DROP INDEX IF EXISTS idx_video_analyses_status;
DROP INDEX IF EXISTS idx_video_analyses_user_id;

DROP TRIGGER IF EXISTS update_video_chunks_updated_at ON video_chunks;
DROP TRIGGER IF EXISTS update_video_analyses_updated_at ON video_analyses;