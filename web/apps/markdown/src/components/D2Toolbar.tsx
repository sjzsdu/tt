import { Button, Slider, Space, Tooltip } from 'antd';
import {
  CopyOutlined,
  DownloadOutlined,
  FileTextOutlined,
  FullscreenExitOutlined,
  ZoomInOutlined,
  ZoomOutOutlined,
} from '@ant-design/icons';

interface D2ToolbarProps {
  scale: number;
  subtitle: string;
  onZoomIn: () => void;
  onZoomOut: () => void;
  onReset: () => void;
  onScaleChange: (scale: number) => void;
  onExportSvg: () => void;
  onExportPng: () => void;
  onCopy: () => void;
}

export function D2Toolbar({
  scale,
  subtitle,
  onZoomIn,
  onZoomOut,
  onReset,
  onScaleChange,
  onExportSvg,
  onExportPng,
  onCopy,
}: D2ToolbarProps) {
  return (
    <div className="d2-toolbar">
      <div className="d2-toolbar-title">
        <strong>D2 diagram</strong>
        <span>{subtitle}</span>
      </div>
      <div className="d2-toolbar-actions">
        <Slider
          min={8}
          max={400}
          value={Math.round(scale * 100)}
          tooltip={{ formatter: v => `${v}%` }}
          className="d2-zoom-slider"
          onChange={v => onScaleChange(v / 100)}
        />
        <Space size={6} wrap>
          <Tooltip title="Zoom Out">
            <Button size="small" icon={<ZoomOutOutlined />} onClick={onZoomOut} />
          </Tooltip>
          <Tooltip title="Fit to view">
            <Button size="small" icon={<FullscreenExitOutlined />} onClick={onReset} />
          </Tooltip>
          <Tooltip title="Zoom In">
            <Button size="small" icon={<ZoomInOutlined />} onClick={onZoomIn} />
          </Tooltip>
          <Tooltip title="Export SVG">
            <Button size="small" icon={<FileTextOutlined />} onClick={onExportSvg} />
          </Tooltip>
          <Tooltip title="Export PNG">
            <Button size="small" icon={<DownloadOutlined />} onClick={onExportPng} />
          </Tooltip>
          <Tooltip title="Copy PNG">
            <Button size="small" icon={<CopyOutlined />} onClick={onCopy} />
          </Tooltip>
        </Space>
      </div>
    </div>
  );
}
