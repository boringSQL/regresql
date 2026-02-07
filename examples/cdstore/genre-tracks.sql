-- name: tracks-by-genre
-- Get the count of tracks by genre
  select genre.name, count(*) as count
    from           genre
         left join track using(genre_id)
group by genre.name
order by count desc;
