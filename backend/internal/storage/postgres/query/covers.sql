-- name: StartCoverRun :one
-- Claims a playlist for a new generation. This is the mutual exclusion that
-- keeps one run in flight per cover, and it has to be the write itself rather
-- than a check in front of one: two requests that both read "idle" and then both
-- wrote would each believe they had won.
--
-- The `WHERE` on the conflict clause is what makes it atomic. A conflicting
-- INSERT ... ON CONFLICT DO UPDATE locks the existing row and re-evaluates that
-- predicate against the *latest* committed version of it, so a second caller
-- either waits for the first and then sees a non-terminal status, or loses the
-- unique-index race and lands on the same predicate. Either way the update is
-- skipped and no row comes back, which is the caller's "already running".
--
-- Upserts on the playlist rather than on the id: a cover is a rendering *of a
-- playlist*, so regenerating claims the identity that already exists instead of
-- adding a second one. The returned id is therefore the accepted id for a first
-- generation and the existing one for a regeneration.
INSERT INTO covers (
    id, user_id, platform, playlist_id, playlist_name, status, error,
    created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (user_id, platform, playlist_id) DO UPDATE SET
    -- The name lookup is best effort, so a run that could not fetch one must not
    -- erase the name an earlier run found.
    playlist_name = COALESCE(NULLIF(EXCLUDED.playlist_name, ''), covers.playlist_name),
    status = EXCLUDED.status,
    -- The previous run's failure is not this one's.
    error = '',
    updated_at = EXCLUDED.updated_at
WHERE covers.status IN ('ready', 'failed')
RETURNING id;

-- name: UpsertCover :one
-- Records a stage change for a run already claimed by StartCoverRun. No status
-- guard: this is the owning pipeline moving its own row forward, including back
-- to a terminal status, which is what releases the claim.
INSERT INTO covers (
    id, user_id, platform, playlist_id, playlist_name, status, error,
    created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (user_id, platform, playlist_id) DO UPDATE SET
    playlist_name = COALESCE(NULLIF(EXCLUDED.playlist_name, ''), covers.playlist_name),
    status = EXCLUDED.status,
    error = EXCLUDED.error,
    updated_at = EXCLUDED.updated_at
RETURNING id;

-- name: GetCoverByID :one
-- The lateral join misses entirely until a run has succeeded, and sqlc cannot
-- see that, so the coalesces are what keep the scan from failing on a NULL. An
-- empty revision_id is the "no artwork yet" signal the mapper reads.
SELECT sqlc.embed(c),
       COALESCE(r.id, '') AS revision_id,
       COALESCE(r.analysis, '{}'::jsonb) AS analysis,
       (SELECT count(*) FROM cover_revisions cr
         WHERE cr.cover_id = c.id AND cr.status = 'ready') AS run_count
FROM covers c
-- The current artwork is "the newest revision that is ready", which during a
-- regeneration is automatically still the previous run's. That is what keeps the
-- tile from blanking, with no promotion step and no pointer to maintain.
LEFT JOIN LATERAL (
    SELECT id, analysis
    FROM cover_revisions
    WHERE cover_id = c.id AND status = 'ready'
    ORDER BY completed_at DESC, id DESC
    LIMIT 1
) r ON true
WHERE c.id = $1;

-- name: ListCoversByUser :many
-- An empty @status returns every lifecycle state; otherwise it filters to one,
-- which reads covers.status and so means "the newest run's outcome".
--
-- Keyset rather than offset: a regeneration reorders a playlist to the front
-- mid-scroll, and offset paging duplicates or skips tiles when that happens.
-- @before is NULL for the first page; id is the tiebreaker because updated_at
-- can collide.
SELECT sqlc.embed(c),
       COALESCE(r.id, '') AS revision_id,
       COALESCE(r.analysis, '{}'::jsonb) AS analysis,
       (SELECT count(*) FROM cover_revisions cr
         WHERE cr.cover_id = c.id AND cr.status = 'ready') AS run_count
FROM covers c
LEFT JOIN LATERAL (
    SELECT id, analysis
    FROM cover_revisions
    WHERE cover_id = c.id AND status = 'ready'
    ORDER BY completed_at DESC, id DESC
    LIMIT 1
) r ON true
WHERE c.user_id = @user_id
  AND (@status::text = '' OR c.status = @status::text)
  AND (
    @before::timestamptz IS NULL
    OR (c.updated_at, c.id) < (@before::timestamptz, @before_id::text)
  )
ORDER BY c.updated_at DESC, c.id DESC
LIMIT @row_limit;

-- name: TouchCover :exec
-- Proof of life for a run in progress, and nothing else.
--
-- Deliberately not an upsert of the whole cover: the pipeline mutates its own
-- domain.Cover as it moves between stages, so a heartbeat that re-saved that
-- struct from another goroutine would both race the pipeline and risk writing
-- back a status the run had already left. The id is all it needs, and the id
-- never changes once the run is claimed.
UPDATE covers SET updated_at = now() WHERE id = $1;

-- name: FailStuckCovers :execrows
-- Generation runs in-process (ADR 0019), so a crash or instance scale-down
-- orphans any in-flight cover in a non-terminal status, and it would hold that
-- playlist's claim forever. A live run heartbeats, so a non-terminal cover
-- untouched since before the cutoff has no worker attached.
--
-- Called two ways: with a cutoff a couple of minutes back, on a timer; and with
-- the current time at startup, which reaps everything in flight because the
-- service runs one instance and a restart means none of them survived.
--
-- No revision is written for one of these, deliberately: nothing was produced.
-- The tile keeps whatever artwork its last successful run left behind.
UPDATE covers
SET status     = 'failed',
    error      = 'generation was interrupted, try again',
    updated_at = now()
WHERE status IN ('pending', 'analyzing', 'generating')
  AND updated_at < @stale_before;

-- name: DeleteCover :execrows
-- Scoped to the owner so a user can never delete another user's cover; the
-- rows-affected count lets the caller distinguish "deleted" from "not found".
-- Every revision, and every revision's image, cascades with it.
DELETE FROM covers
WHERE id = $1 AND user_id = $2;

-- name: UpsertCoverRevision :exec
-- Written once, when a run reaches a terminal status. An upsert because the
-- failure path re-writes a revision that had already been recorded as ready:
-- storing the image bytes is the last thing that can go wrong, and it happens
-- after the row exists (the bytes are keyed by revision id and cascade with it).
--
-- The number is taken here, not by the caller: it is MAX + 1 within the cover,
-- and only one run per cover is ever in flight, so nothing else can be taking
-- the same one. It is deliberately absent from the DO UPDATE list, because the
-- re-write above is the same run correcting its own outcome and must not
-- renumber itself.
INSERT INTO cover_revisions (
    id, cover_id, revision_number, status, prompt, analysis, error, completed_at
)
VALUES (
    $1, $2,
    (SELECT COALESCE(max(revision_number), 0) + 1
       FROM cover_revisions WHERE cover_id = $2),
    $3, $4, $5, $6, $7
)
ON CONFLICT (id) DO UPDATE SET
    status = EXCLUDED.status,
    prompt = EXCLUDED.prompt,
    analysis = EXCLUDED.analysis,
    error = EXCLUDED.error,
    completed_at = EXCLUDED.completed_at;

-- name: ListCoverRevisions :many
-- Failed runs are recorded but hidden unless asked for: their error is what
-- explains an image-provider refusal, but the run history is about renders that
-- exist.
SELECT id, cover_id, revision_number, status, prompt, analysis, error, completed_at
FROM cover_revisions
WHERE cover_id = @cover_id
  AND (@include_failed::bool OR status = 'ready')
ORDER BY completed_at DESC, id DESC;

-- name: DeleteCoverRevision :execrows
-- Scoped to the owning cover *and* user, so neither id can be used to reach
-- someone else's run. The revision's image cascades with it.
DELETE FROM cover_revisions r
USING covers c
WHERE r.id = @revision_id
  AND r.cover_id = c.id
  AND c.id = @cover_id
  AND c.user_id = @user_id;
