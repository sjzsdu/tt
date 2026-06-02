import { Button, Card, Empty, Timeline, Typography } from 'antd';
import type { FormulaDashboardLogEntry, FormulaDashboardStep } from '../../types';

function matchLogStep(log: FormulaDashboardLogEntry, steps: FormulaDashboardStep[]) {
  const text = log.text.toLowerCase();
  return steps.find(step => text.includes(step.id.toLowerCase()))
    || steps.find(step => step.title && text.includes(step.title.toLowerCase()))
    || null;
}

export function ExecutionTimeline({ logs, steps, onSelectStep }: { logs: FormulaDashboardLogEntry[]; steps: FormulaDashboardStep[]; onSelectStep: (step: FormulaDashboardStep) => void }) {
  return (
    <Card className="console-card" title="Execution timeline">
      {logs.length ? (
        <Timeline
          items={logs.slice(-14).map(log => {
            const isError = /fail|error|blocked/i.test(log.text);
            const matchedStep = matchLogStep(log, steps);
            return {
              color: isError ? 'red' : matchedStep ? 'green' : 'blue',
              children: (
                <div className={isError ? 'timeline-entry timeline-entry-error' : 'timeline-entry'}>
                  <Typography.Text type="secondary" className="timeline-time">{log.at}</Typography.Text>
                  <Typography.Paragraph className="timeline-text">{log.text}</Typography.Paragraph>
                  {matchedStep && <Button size="small" onClick={() => onSelectStep(matchedStep)}>Open step</Button>}
                </div>
              ),
            };
          })}
        />
      ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No timeline events yet" />}
    </Card>
  );
}
