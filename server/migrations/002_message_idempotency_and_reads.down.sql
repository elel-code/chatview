DROP INDEX IF EXISTS idx_conversation_reads_user;
DROP TABLE IF EXISTS conversation_reads;
DROP INDEX IF EXISTS idx_messages_sender_client_message;
ALTER TABLE messages DROP COLUMN IF EXISTS client_message_id;
