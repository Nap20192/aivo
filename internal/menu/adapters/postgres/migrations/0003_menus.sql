-- Multiple menus per restaurant (dinner, lunch, bar...). Exactly one
-- default menu per restaurant; categories move under menus.

CREATE TABLE menus (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    slug          text NOT NULL,
    name          text NOT NULL,
    position      integer NOT NULL DEFAULT 0,
    is_default    boolean NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX menus_restaurant_slug_idx ON menus (restaurant_id, slug);

-- Exactly one default per restaurant.
CREATE UNIQUE INDEX menus_default_per_restaurant_idx
    ON menus (restaurant_id)
    WHERE is_default;

ALTER TABLE categories
    ADD COLUMN menu_id uuid REFERENCES menus (id) ON DELETE CASCADE;

-- Every existing restaurant gets its default menu ("menu" / "Menu") and
-- its categories move into it.
INSERT INTO menus (id, restaurant_id, slug, name, position, is_default)
SELECT gen_random_uuid(), id, 'menu', 'Menu', 0, true FROM restaurants;

UPDATE categories c
SET menu_id = m.id
FROM menus m
WHERE m.restaurant_id = c.restaurant_id AND m.is_default;

ALTER TABLE categories ALTER COLUMN menu_id SET NOT NULL;

CREATE INDEX categories_menu_id_idx ON categories (menu_id);
