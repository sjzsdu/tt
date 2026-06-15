#!/bin/bash
set -euo pipefail
jq -cn \
  --arg metadata "${TT_METADATA:-}" \
  --arg prepare "${TT_PREPARE:-}" \
  --arg attempt "${TT_ATTEMPT:-}" \
  --arg resolve "${TT_RESOLVE:-}" \
  --arg cont "${TT_CONTINUE:-}" '
  def obj($s): if $s == null or ($s|length)==0 then {} else (($s|fromjson?) // {}) end;
  obj($metadata) as $m |
  obj($prepare) as $p |
  obj($attempt) as $a |
  obj($resolve) as $r |
  obj($cont) as $c |
  (($a.success == true) or ($c.success == true)) as $done |
  (($m.ok == false) and (($m.error // "") != "")) as $metadata_failed |
  (($p.ok == false) and (($p.error // "") != "")) as $prepare_failed |
  (($r.blocker // "") != "") as $resolver_blocked |
  (($a.conflict == true) or ($a.in_rebase == true)) as $rebase_interrupted |
  {
    rebase_done:$done,
    conflict_seen:$rebase_interrupted,
    blocked:(($done|not) and ($metadata_failed or $prepare_failed or $rebase_interrupted or $resolver_blocked or (($a.error // "") != "") or (($c.error // "") != ""))),
    blocker:(
      if $done then ""
      elif $metadata_failed then ("PR metadata unavailable: " + ($m.error // ""))
      elif $prepare_failed then ("prepare rebase worktree failed: " + ($p.error // ""))
      elif $resolver_blocked then ($r.blocker // "")
      elif ($c.error // "") != "" then $c.error
      elif ($a.error // "") != "" then $a.error
      else "rebase did not complete" end
    ),
    head:($c.head // $a.head // $p.head_sha // ""),
    conflict_files:($c.conflict_files // $r.remaining_conflicts // $a.conflict_files // [])
  }'
