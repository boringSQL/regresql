-- Idempotent category loading using upsert
INSERT INTO categories (id, code, name)
OVERRIDING SYSTEM VALUE
VALUES
  (1, 'ELEC', 'Electronics'),
  (2, 'FURN', 'Furniture'),
  (3, 'CLTH', 'Clothing'),
  (4, 'FOOD', 'Food'),
  (5, 'TOYS', 'Toys')
ON CONFLICT (id) DO UPDATE
  SET code = EXCLUDED.code,
      name = EXCLUDED.name;
