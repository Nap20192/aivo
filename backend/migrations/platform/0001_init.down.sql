DROP TABLE IF EXISTS custom_domains, restaurant_themes, subscriptions,
    sessions, users CASCADE;
ALTER TABLE restaurants
    DROP COLUMN IF EXISTS org_id,
    DROP COLUMN IF EXISTS address,
    DROP COLUMN IF EXISTS hours,
    DROP COLUMN IF EXISTS contacts;
DROP TABLE IF EXISTS organizations CASCADE;
