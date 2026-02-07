-- name: list-albums-by-artist
-- List the album titles and duration of a given artist
  select album.title as album,
         created_at,
         sum(milliseconds) * interval '1 ms' as duration
    from album
         join artist using(artist_id)
         left join track using(album_id)
   where artist.name = :name
group by album, created_at
order by album;
