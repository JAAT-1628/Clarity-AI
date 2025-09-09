CREATE TABLE subscriptions_backup AS SELECT * FROM subscriptions WHERE EXISTS (SELECT 1 FROM subscriptions LIMIT 1);
DROP TABLE IF EXISTS subscriptions;

CREATE TABLE subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stripe_customer_id VARCHAR(255),
    stripe_subscription_id VARCHAR(255) UNIQUE,
    plan VARCHAR(20) NOT NULL CHECK (plan IN ('free', 'premium', 'pro')) DEFAULT 'free',
    status VARCHAR(50) NOT NULL DEFAULT 'inactive',
    current_period_end TIMESTAMP,
    cancel_at_period_end BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO subscriptions (id, user_id, plan, status, created_at, updated_at)
SELECT 
    COALESCE(id, gen_random_uuid()) as id,
    user_id,
    COALESCE(plan, 'free') as plan,
    CASE 
        WHEN status = 'cancelled' THEN 'canceled'
        WHEN status = 'on_trial' THEN 'trialing'
        ELSE COALESCE(status, 'inactive')
    END as status,
    COALESCE(created_at, CURRENT_TIMESTAMP) as created_at,
    COALESCE(updated_at, CURRENT_TIMESTAMP) as updated_at
FROM subscriptions_backup
WHERE EXISTS (SELECT 1 FROM subscriptions_backup LIMIT 1);

DROP TABLE IF EXISTS subscriptions_backup;

CREATE INDEX idx_subscriptions_user_id ON subscriptions(user_id);
CREATE INDEX idx_subscriptions_stripe_customer_id ON subscriptions(stripe_customer_id);
CREATE INDEX idx_subscriptions_stripe_subscription_id ON subscriptions(stripe_subscription_id);
CREATE INDEX idx_subscriptions_status ON subscriptions(status);

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_subscriptions_updated_at
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
