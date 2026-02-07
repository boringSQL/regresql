-- name: genre-top-n
-- Get the N top tracks by genre
select genre.name as genre,
       case when length(ss.name) > 15
            then substring(ss.name from 1 for 15) || '…'
            else ss.name
        end as track,
       artist.name as artist
  from genre
       left join lateral
       /*
        * the lateral left join implements a nested loop over
        * the genres and allows to fetch our Top-N tracks per
        * genre, applying the order by desc limit n clause.
        *
        * here we choose to weight the tracks by how many
        * times they appear in a playlist, so we join against
        * the playlist_track table and count appearances.
        */
       (
          select track.name, track.album_id, count(playlist_id)
            from           track
                 left join playlist_track using (track_id)
           where track.genre_id = genre.genre_id
        group by track.track_id
        order by count desc
           limit :n
       )
       /*
        * the join happens in the subquery's where clause, so
        * we don't need to add another one at the outer join
        * level, hence the "on true" spelling.
        */
            ss(name, album_id, count) on true
       join album using(album_id)
       join artist using(artist_id)
order by genre.name, ss.count desc;
