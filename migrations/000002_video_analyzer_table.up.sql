CREATE TABLE video_analyses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_url VARCHAR(2048) NOT NULL,
    video_title VARCHAR(500) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    ai_provider VARCHAR(50) NOT NULL,
    total_chunks INTEGER DEFAULT 0,
    completed_chunks INTEGER DEFAULT 0,
    failed_chunks INTEGER DEFAULT 0,
    total_duration_minutes INTEGER DEFAULT 0,
    processing_options JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE TABLE video_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    analysis_id UUID NOT NULL REFERENCES video_analyses(id) ON DELETE CASCADE,
    chunk_number INTEGER NOT NULL,
    start_time VARCHAR(20) NOT NULL,
    end_time VARCHAR(20) NOT NULL,  
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    transcript TEXT,
    summary TEXT,
    processing_stage VARCHAR(50) DEFAULT 'pending',
    progress_percentage INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    UNIQUE(analysis_id, chunk_number)
);

CREATE TABLE topics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chunk_id UUID NOT NULL REFERENCES video_chunks(id) ON DELETE CASCADE,
    analysis_id UUID NOT NULL REFERENCES video_analyses(id) ON DELETE CASCADE,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    key_points JSONB DEFAULT '[]',
    timestamp_start VARCHAR(20),
    timestamp_end VARCHAR(20),
    why_important TEXT,
    related_concepts JSONB DEFAULT '[]',
    difficulty VARCHAR(50) DEFAULT 'beginner',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE questions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic_id UUID REFERENCES topics(id) ON DELETE CASCADE,
    chunk_id UUID NOT NULL REFERENCES video_chunks(id) ON DELETE CASCADE,
    analysis_id UUID NOT NULL REFERENCES video_analyses(id) ON DELETE CASCADE,
    question TEXT NOT NULL,
    answer TEXT NOT NULL,
    difficulty VARCHAR(50) DEFAULT 'easy',
    type VARCHAR(50) DEFAULT 'multiple_choice',
    timestamp_reference VARCHAR(20),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE key_insights (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chunk_id UUID NOT NULL REFERENCES video_chunks(id) ON DELETE CASCADE,
    analysis_id UUID NOT NULL REFERENCES video_analyses(id) ON DELETE CASCADE,
    insight TEXT NOT NULL,
    explanation TEXT,
    timestamp VARCHAR(20),
    type VARCHAR(50) DEFAULT 'key_concept',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_video_analyses_user_id ON video_analyses(user_id);
CREATE INDEX idx_video_analyses_status ON video_analyses(status);
CREATE INDEX idx_video_chunks_analysis_id ON video_chunks(analysis_id);
CREATE INDEX idx_video_chunks_status ON video_chunks(status);
CREATE INDEX idx_topics_analysis_id ON topics(analysis_id);
CREATE INDEX idx_questions_analysis_id ON questions(analysis_id);
CREATE INDEX idx_key_insights_analysis_id ON key_insights(analysis_id);

CREATE TRIGGER update_video_analyses_updated_at
    BEFORE UPDATE ON video_analyses
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_video_chunks_updated_at
    BEFORE UPDATE ON video_chunks
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();