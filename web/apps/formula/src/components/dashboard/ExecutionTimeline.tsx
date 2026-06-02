import { Card, Empty, Timeline, Typography } from 'antd';
import type { FormulaDashboardLogEntry } from '../../types';

export function ExecutionTimeline({ logs }: { logs: FormulaDashboardLogEntry[] }) {
  return (
    <Card className="console-card" title="Execution timeline">
      {logs.length ? (
        <Timeline
          items={logs.slice(-14).map(log => {
            const isError = /fail|error|blocked/i.test(log.text);
            return {
              color: isError ? 'red' : 'blue',
              children: (
                <div className={isError ? 'timeline-entry timeline-entry-error' : 'timeline-entry'}>
                  <Typography.Text type="secondary" className="timeline-time">{log.at}</Typography.Text>
                  <Typography.Paragraph className="timeline-text">{log.text}</Typography.Paragraph>
                </div>
              ),
            };
          })}
        />
      ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No timeline events yet" />}
    </Card>
  );
}
