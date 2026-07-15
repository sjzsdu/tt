import type {
  FormulaDashboardSnapshot,
  FormulaDashboardStep,
  FormulaExecutionEvent,
  FormulaExecutionInstance,
  FormulaStepActivity,
} from '../types';

const ACTIVE_STATUSES = new Set(['running', 'waiting_input']);
const TERMINAL_STATUSES = new Set(['completed', 'failed', 'skipped', 'interrupted']);

export function parseExecutionAddress(address: string) {
  const matches = [...address.matchAll(/\.iter(\d+)\./g)];
  if (!matches.length) return { parentLoopID: '', iterationPath: [] as number[], bodyStepID: '' };
  const last = matches.at(-1)!;
  return {
    parentLoopID: address.slice(0, matches[0].index),
    iterationPath: matches.map(match => Number.parseInt(match[1], 10)).filter(Number.isFinite),
    bodyStepID: address.slice((last.index || 0) + last[0].length),
  };
}

function executionPathMetadata(instance: FormulaExecutionInstance) {
  if (!instance.path?.length) return {};
  return {
    execution_path: instance.path.map(segment => segment.kind === 'iteration'
      ? `iter:${segment.index}`
      : `${segment.kind}:${segment.id || ''}`).join(' / '),
    formula_path: instance.formula_path?.join(' / ') || '',
  };
}

export function iterationLabel(path?: number[]) {
  if (!path?.length) return '';
  return path.map((value, index) => `${index ? 'nested ' : ''}iter ${value}`).join(' / ');
}

export function executionAddressLabel(instance: FormulaExecutionInstance) {
	if (instance.path?.length) {
		return instance.path.map(segment => {
			if (segment.kind === 'formula') return `formula ${segment.id || ''}`;
			if (segment.kind === 'iteration') return `iter ${segment.index || 0}`;
			return segment.id || '';
		}).filter(Boolean).join(' / ');
	}
  const iteration = iterationLabel(instance.iteration_path);
  return [instance.parent_loop_id, iteration, instance.body_step_id || instance.definition_step_id]
    .filter(Boolean)
    .join(' / ') || instance.address;
}

export function executionFormulaLabel(instance: FormulaExecutionInstance) {
	return instance.formula_path?.join(' / ') || '';
}

export function executionRootStepID(instance: FormulaExecutionInstance) {
	const root = instance.path?.find(segment => segment.kind === 'step' && segment.id)?.id;
	if (root) return root;
	return instance.address.split('.formula(')[0].split('.iter')[0];
}

function fromActivity(activity: FormulaStepActivity, parent: FormulaDashboardStep): FormulaExecutionInstance {
  const parsed = parseExecutionAddress(activity.step_id);
  return {
    address: activity.step_id,
    definition_step_id: parsed.bodyStepID || activity.step_id,
    parent_loop_id: parsed.parentLoopID || undefined,
    body_step_id: parsed.bodyStepID || undefined,
    iteration_path: parsed.iterationPath,
    title: activity.title || activity.step_id,
    status: activity.status,
    updated_at: activity.at,
    duration_ms: activity.duration_ms,
    session: activity.session,
    detail: activity.detail,
    output: activity.output,
    error: activity.error,
  };
}

function topLevelInstance(step: FormulaDashboardStep): FormulaExecutionInstance {
  return {
    address: step.id,
    definition_step_id: step.id,
    title: step.title,
    status: step.status,
    started_at: step.started_at,
    finished_at: step.finished_at,
    updated_at: step.finished_at || step.started_at,
    duration_ms: step.duration_ms,
    session: step.session,
    output: step.output,
    error: step.error,
  };
}

export function executionInstances(snapshot: FormulaDashboardSnapshot): FormulaExecutionInstance[] {
  if (snapshot.execution_instances?.length) return snapshot.execution_instances;
  const instances = new Map<string, FormulaExecutionInstance>();
  for (const step of snapshot.steps) {
    const visible = step.status !== 'pending' || step.started_at || step.finished_at || step.activities?.length;
    if (visible) instances.set(step.id, topLevelInstance(step));
    for (const activity of step.activities || []) {
      instances.set(activity.step_id, fromActivity(activity, step));
    }
  }
  return [...instances.values()];
}

export function executionEvents(snapshot: FormulaDashboardSnapshot): FormulaExecutionEvent[] {
  if (snapshot.execution_events?.length) return snapshot.execution_events;
  const events: FormulaExecutionEvent[] = [];
  for (const step of snapshot.steps) {
    for (const activity of step.activities || []) {
      const parsed = parseExecutionAddress(activity.step_id);
      events.push({
        id: `legacy-${activity.step_id}-${activity.at}-${activity.status}`,
        at: activity.at,
        instance_address: activity.step_id,
        definition_step_id: parsed.bodyStepID || activity.step_id,
        parent_loop_id: parsed.parentLoopID || undefined,
        type: activity.status,
        status: activity.status,
        title: activity.title,
        detail: activity.detail,
        duration_ms: activity.duration_ms,
        session: activity.session,
        error: activity.error,
      });
    }
  }
  if (events.length) return events;
  return snapshot.logs.map((log, index) => ({
    id: `log-${index}-${log.at}`,
    at: log.at,
    type: /fail|error|blocked/i.test(log.text) ? 'failed' : 'log',
    status: /fail|error|blocked/i.test(log.text) ? 'failed' : 'log',
    detail: log.text,
  }));
}

export function isActiveExecution(instance: FormulaExecutionInstance) {
  return ACTIVE_STATUSES.has(instance.status);
}

export function isTerminalExecution(instance: FormulaExecutionInstance) {
  return TERMINAL_STATUSES.has(instance.status);
}

export function executionUpdatedAt(instance: FormulaExecutionInstance) {
  return instance.updated_at || instance.finished_at || instance.started_at || '';
}

export function executionInstanceStep(instance: FormulaExecutionInstance, snapshot: FormulaDashboardSnapshot): FormulaDashboardStep {
  const parent = snapshot.steps.find(step => step.id === executionRootStepID(instance))
    || snapshot.steps.find(step => step.id === instance.parent_loop_id)
    || snapshot.steps.find(step => step.id === instance.definition_step_id);
  return {
    id: instance.address,
    title: instance.title || instance.body_step_id || instance.definition_step_id,
    description: parent?.description,
	type: instance.parent_loop_id || instance.formula_path?.length ? 'execution-instance' : parent?.type,
    agent: parent?.agent || '',
    model: parent?.model,
    session: instance.session,
    status: instance.status,
    output: instance.output,
    error: instance.error,
    started_at: instance.started_at,
    finished_at: instance.finished_at,
    duration_ms: instance.duration_ms,
    execution: parent?.execution,
    metadata: {
      ...(parent?.metadata || {}),
      ...executionPathMetadata(instance),
      runtime_address: instance.address,
      parent_loop: instance.parent_loop_id || '',
		formula_path: executionFormulaLabel(instance),
      iteration: iterationLabel(instance.iteration_path),
      definition_step: instance.definition_step_id,
    },
    activities: [{
      at: executionUpdatedAt(instance), step_id: instance.address,
      title: instance.title, status: instance.status, session: instance.session,
      detail: instance.detail, output: instance.output, error: instance.error,
      duration_ms: instance.duration_ms,
    }],
		depth: (instance.iteration_path?.length || 0) + (instance.formula_path?.length || 0),
    index: parent?.index || 0,
  };
}

export function compareExecutionRecency(a: FormulaExecutionInstance, b: FormulaExecutionInstance) {
  return executionUpdatedAt(b).localeCompare(executionUpdatedAt(a)) || b.address.localeCompare(a.address);
}
