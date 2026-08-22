PostgreSQL cu tabela jobs.
Un POST /jobs care doar salvează un job ca pending.
Un singur worker care caută un job pending, îl procesează și îl pune completed.
Abia după ce asta merge, faci mai mulți workeri.
După aceea adaugi FOR UPDATE SKIP LOCKED.
Apoi retry.
La final context + graceful shutdown.
