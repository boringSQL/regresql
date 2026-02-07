-- name: list-tracks-by-albumid
-- List the tracks of an album, includes duration and position
   select name as title,
          milliseconds * interval '1ms' as duration,
          (sum(milliseconds) over (order by track_id) - milliseconds)
            * interval '1ms' as "begin",
          sum(milliseconds) over (order by track_id)
            * interval '1ms' as "end",
          round(milliseconds / sum(milliseconds) over () * 100, 2) as pct
    from track
   where album_id = :album_id
order by track_id;
