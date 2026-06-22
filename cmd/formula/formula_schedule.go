package formulacmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/adhocore/gronx"
	"github.com/spf13/cobra"
)

type formulaSchedulePlan struct {
	Every time.Duration
	Cron  string
}

func runFormulaSchedule(cmd *cobra.Command, args []string) error {
	plan, err := parseFormulaSchedulePlan(formulaScheduleEvery, formulaScheduleCron)
	if err != nil {
		return err
	}
	if formulaScheduleMaxRuns < 0 {
		return fmt.Errorf("--max-runs cannot be negative")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Scheduling formula %q", args[0])
	if plan.Every > 0 {
		fmt.Fprintf(out, " every %s", plan.Every)
	} else {
		fmt.Fprintf(out, " with cron %q", plan.Cron)
	}
	if formulaScheduleMaxRuns > 0 {
		fmt.Fprintf(out, " for %d run(s)", formulaScheduleMaxRuns)
	}
	fmt.Fprintln(out, ". Press Ctrl-C to stop.")

	runs := 0
	runOnce := func() error {
		runs++
		started := time.Now()
		fmt.Fprintf(out, "\n[%s] Starting scheduled formula run #%d\n", started.Format(time.RFC3339), runs)
		oldNoWeb := formulaNoWeb
		oldWeb := formulaWeb
		formulaNoWeb = !formulaScheduleWeb
		formulaWeb = formulaScheduleWeb
		err := runFormulaRun(cmd, args)
		formulaNoWeb = oldNoWeb
		formulaWeb = oldWeb
		if err != nil {
			fmt.Fprintf(out, "[%s] Scheduled formula run #%d failed: %v\n", time.Now().Format(time.RFC3339), runs, err)
			return err
		}
		fmt.Fprintf(out, "[%s] Finished scheduled formula run #%d in %s\n", time.Now().Format(time.RFC3339), runs, time.Since(started).Round(time.Second))
		return nil
	}

	if formulaScheduleRunNow {
		if err := runOnce(); err != nil && formulaScheduleStopOnError {
			return err
		}
		if reachedFormulaScheduleMaxRuns(runs, formulaScheduleMaxRuns) {
			return nil
		}
	}

	for {
		next, err := nextFormulaScheduleTick(plan, time.Now())
		if err != nil {
			return err
		}
		wait := time.Until(next)
		if wait < 0 {
			wait = 0
		}
		fmt.Fprintf(out, "[%s] Next run at %s\n", time.Now().Format(time.RFC3339), next.Format(time.RFC3339))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			fmt.Fprintln(out, "Scheduler stopped.")
			return nil
		case <-timer.C:
		}

		if err := runOnce(); err != nil && formulaScheduleStopOnError {
			return err
		}
		if reachedFormulaScheduleMaxRuns(runs, formulaScheduleMaxRuns) {
			return nil
		}
	}
}

func parseFormulaSchedulePlan(every, cronExpr string) (formulaSchedulePlan, error) {
	every = strings.TrimSpace(every)
	cronExpr = strings.TrimSpace(cronExpr)
	if every == "" && cronExpr == "" {
		return formulaSchedulePlan{}, fmt.Errorf("one of --every or --cron is required")
	}
	if every != "" && cronExpr != "" {
		return formulaSchedulePlan{}, fmt.Errorf("--every and --cron are mutually exclusive")
	}
	if every != "" {
		d, err := time.ParseDuration(every)
		if err != nil {
			return formulaSchedulePlan{}, fmt.Errorf("parse --every: %w", err)
		}
		if d <= 0 {
			return formulaSchedulePlan{}, fmt.Errorf("--every must be greater than zero")
		}
		return formulaSchedulePlan{Every: d}, nil
	}
	if !gronx.IsValid(cronExpr) {
		return formulaSchedulePlan{}, fmt.Errorf("invalid --cron expression %q; expected a 5-field crontab expression like '*/2 * * * *'", cronExpr)
	}
	return formulaSchedulePlan{Cron: cronExpr}, nil
}

func nextFormulaScheduleTick(plan formulaSchedulePlan, now time.Time) (time.Time, error) {
	if plan.Every > 0 {
		return now.Add(plan.Every), nil
	}
	if strings.TrimSpace(plan.Cron) == "" {
		return time.Time{}, fmt.Errorf("schedule plan has no interval or cron expression")
	}
	return gronx.NextTickAfter(plan.Cron, now.Add(time.Minute).Truncate(time.Minute), true)
}

func reachedFormulaScheduleMaxRuns(runs, maxRuns int) bool {
	return maxRuns > 0 && runs >= maxRuns
}
