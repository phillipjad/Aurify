-- +goose Up
-- Push for cover lifecycle changes (see docs/adr/0019-async-cover-generation.md).
-- A trigger rather than application code, so every writer fires it: the
-- generation pipeline, the stuck-cover sweep, and anything added later. The
-- payload is just the id; listeners re-read the row, so a notification can
-- never carry stale state.

-- +goose StatementBegin
CREATE FUNCTION notify_cover_update() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('cover_updates', NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER covers_notify_update
AFTER INSERT OR UPDATE ON covers
FOR EACH ROW EXECUTE FUNCTION notify_cover_update();

-- +goose Down
DROP TRIGGER covers_notify_update ON covers;
DROP FUNCTION notify_cover_update;
