-- name: top-artists-by-album
-- Get the list of the N artists with the most albums
  select artist.name, count(*) as albums
    from           artist
         left join album using(artist_id)
group by artist.name
order by albums asc
   limit :n;
