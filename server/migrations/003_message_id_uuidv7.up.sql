DO $$
DECLARE
    id_type TEXT;
BEGIN
    SELECT data_type INTO id_type
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'messages'
      AND column_name = 'id';

    IF id_type = 'text' THEN
        IF EXISTS (
            SELECT 1
            FROM messages
            WHERE id !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
        ) THEN
            RAISE EXCEPTION 'cannot convert messages.id to UUID: existing non-UUID message IDs found';
        END IF;

        ALTER TABLE messages
            ALTER COLUMN id DROP DEFAULT,
            ALTER COLUMN id TYPE UUID USING id::uuid,
            ALTER COLUMN id SET DEFAULT uuidv7();
    ELSIF id_type = 'uuid' THEN
        ALTER TABLE messages
            ALTER COLUMN id SET DEFAULT uuidv7();
    END IF;
END $$;
