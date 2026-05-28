import { Tag } from 'antd';
import type { FormulaDashboardStep } from '../../types';
import { activityShortId, statusIcon, statusLabel, statusTone } from '../../utils/status';
import { loopActivitySummary } from '../../utils/steps';

export function StepCard({ step, onSelect }: { step: FormulaDashboardStep; onSelect: (step: FormulaDashboardStep) => void }) {
  const latest = step.activities?.at(-1);
  const loopSummary = loopActivitySummary(step);
  return (
    <button type="button" className={`step-card ${step.status}`} onClick={() => onSelect(step)}>
      <div className="step-card-row">
        <div>
          <div className="step-card-kicker">{step.id}</div>
          <h3>{step.title}</h3>
        </div>
        <Tag color={statusTone[step.status] || 'default'} icon={statusIcon(step.status)}>{step.status}</Tag>
      </div>
      <p>{step.description || step.notes || 'No extra description for this step.'}</p>
      {loopSummary && <div className="loop-summary-pill">↻ {loopSummary}</div>}
      {latest && (
        <div className={`step-activity-mini ${latest.status}`}>
          <span>{latest.at}</span>{activityShortId(latest.step_id)} · {latest.title || statusLabel(latest.status)}
        </div>
      )}
      <div className="step-chip-row">
        {step.agent && <span className="step-chip">agent · {step.agent}</span>}
        {step.model && <span className="step-chip">model · {step.model}</span>}
        {!!step.depends_on?.length && <span className="step-chip">deps · {step.depends_on.length}</span>}
        {step.loop && <span className="step-chip loop-chip">loop · {step.loop.body?.length || 0} body</span>}
        {!!step.activities?.length && <span className="step-chip">activity · {step.activities.length}</span>}
        {step.output_key && <span className="step-chip">output · {step.output_key}</span>}
      </div>
    </button>
  );
}
