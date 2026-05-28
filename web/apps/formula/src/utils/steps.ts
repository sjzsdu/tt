import type { FormulaDashboardSnapshot, FormulaDashboardStep, FormulaStepActivity } from '../types';
import { statusLabel } from './status';

export function loopActivitySummary(step: FormulaDashboardStep) {
  const activities = step.activities || [];
  const iterations = new Set<string>();
  for (const activity of activities) {
    const match = activity.step_id.match(/\.iter(\d+)\./);
    if (match) iterations.add(match[1]);
  }
  if (!step.loop && !activities.length) return '';
  const bodyCount = step.loop?.body?.length || 0;
  const bits = [];
  if (step.loop?.summary) bits.push(step.loop.summary);
  if (bodyCount) bits.push(`${bodyCount} body step${bodyCount === 1 ? '' : 's'}`);
  if (iterations.size) bits.push(`${iterations.size} iteration${iterations.size === 1 ? '' : 's'} seen`);
  return bits.join(' · ');
}

export function latestLoopActivity(step: FormulaDashboardStep) {
  return [...(step.activities || [])].reverse().find(activity => /\.iter\d+\./.test(activity.step_id));
}

export function loopActivityIteration(activity?: { step_id: string }) {
  return activity?.step_id.match(/\.iter(\d+)\./)?.[1] || '';
}

export function loopActivityBodyID(activity?: { step_id: string }) {
  return activity?.step_id.match(/\.iter\d+\.([^.]*)$/)?.[1] || '';
}

export function groupLoopActivities(activities: FormulaStepActivity[]) {
  const groups: Array<{ iteration: string; activities: FormulaStepActivity[] }> = [];
  const byIteration = new Map<string, FormulaStepActivity[]>();
  for (const activity of activities) {
    const iteration = loopActivityIteration(activity) || 'step';
    if (!byIteration.has(iteration)) {
      byIteration.set(iteration, []);
      groups.push({ iteration: iteration === 'step' ? '' : iteration, activities: byIteration.get(iteration)! });
    }
    byIteration.get(iteration)!.push(activity);
  }
  return groups;
}

export function sameOutput(a?: string, b?: string) {
  return !!a && !!b && a.trim() === b.trim();
}

export function collectStepInputValues(step: FormulaDashboardStep, snapshot: FormulaDashboardSnapshot | null) {
  if (!snapshot) return [];
  const byID = new Map(snapshot.steps.map(item => [item.id, item]));
  const keys: Array<{ key: string; source: string }> = [];
  const seen = new Set<string>();
  const add = (key?: string, source = 'context') => {
    const clean = (key || '').trim();
    if (!clean || seen.has(clean)) return;
    seen.add(clean);
    keys.push({ key: clean, source });
  };
  step.input_ctx?.forEach(key => add(key, 'input_context'));
  step.depends_on?.forEach(key => add(key, 'dependency'));
  if (step.loop?.for_each) add(step.loop.for_each, 'for_each');

  return keys.map(({ key, source }) => {
    const root = key.split('.')[0];
    const sourceStep = byID.get(root);
    const value = resolveStepInputValue(key, sourceStep?.output);
    return value ? { key, source, value } : null;
  }).filter((item): item is { key: string; source: string; value: string } => !!item);
}

function resolveStepInputValue(key: string, output?: string) {
  if (!output) return '';
  const parts = key.split('.').slice(1);
  if (!parts.length) return output;
  try {
    let current: unknown = JSON.parse(output);
    for (const part of parts) {
      if (!current || typeof current !== 'object' || !(part in current)) return output;
      current = (current as Record<string, unknown>)[part];
    }
    return typeof current === 'string' ? current : JSON.stringify(current, null, 2);
  } catch {
    return output;
  }
}

export function attentionStep(snapshot: FormulaDashboardSnapshot) {
  return snapshot.steps.find(step => step.status === 'waiting_input')
    || snapshot.steps.find(step => step.status === 'failed')
    || snapshot.steps.find(step => step.status === 'running')
    || null;
}

export function attentionCopy(step: FormulaDashboardStep | null, status: string) {
  if (!step) {
    if (status === 'completed') return { title: 'Run completed', detail: 'Open the final report or inspect completed steps.', tone: 'completed' };
    return { title: 'No active step', detail: 'The run is waiting for the scheduler or next update.', tone: 'pending' };
  }
  if (step.status === 'waiting_input') return { title: 'Input required', detail: step.title, tone: 'waiting_input' };
  if (step.status === 'failed') return { title: 'Action needed', detail: step.title, tone: 'failed' };
  if (step.status === 'running') return { title: 'Currently running', detail: step.title, tone: 'running' };
  return { title: statusLabel(step.status), detail: step.title, tone: step.status };
}
