import React, { useState, useRef, useEffect, useCallback } from 'react';
import { Icon } from '../../components/Icon';
import { useMultiSelection, useMultiSetting, multiRunCommand } from '../../multi';

interface MovementPadProps {
  onMoveVector?: (dx: number, dz: number) => void;
  onStop?: () => void;
  onDoor?: () => void;
  stepSize?: number;
  onStepSizeChange?: (val: number) => void;
}

const MAX_RADIUS = 38; // Max thumbstick travel distance from center (px)
const BASE_SIZE = 124; // Joystick track diameter (px)

export const MovementPad: React.FC<MovementPadProps> = ({
  onMoveVector,
  onStop,
  onDoor,
  stepSize: propStepSize,
  onStepSizeChange,
}) => {
  const { target } = useMultiSelection();
  const { value: stepPref, setValue: setStepPref } = useMultiSetting<number>(
    'main.preferences.move_step',
    propStepSize ?? 1.2
  );

  const [position, setPosition] = useState<{ x: number; y: number }>({ x: 0, y: 0 });
  const [isDragging, setIsDragging] = useState(false);
  const [localStepSize, setLocalStepSize] = useState<number>(propStepSize ?? 1.2);
  const [statusText, setStatusText] = useState<string>('Idle');

  const stepSize = typeof stepPref === 'number' ? stepPref : (propStepSize ?? localStepSize);
  const updateStep = (val: number) => {
    const clamped = Math.min(20, Math.max(0.2, Number(val.toFixed(1))));
    setLocalStepSize(clamped);
    void setStepPref(clamped);
    onStepSizeChange?.(clamped);
  };

  const baseRef = useRef<HTMLDivElement>(null);
  const activePointerId = useRef<number | null>(null);
  const moveTimerRef = useRef<number | null>(null);
  const vectorRef = useRef<{ dx: number; dz: number }>({ dx: 0, dz: 0 });

  // Update vector and direction status
  const updateDirection = useCallback((normX: number, normY: number, step: number) => {
    // Coordinate mapping:
    // Screen X right (+) -> Game X (+) (East)
    // Screen X left  (-) -> Game X (-) (West)
    // Screen Y up    (-) -> Game Z (-) (North / Forward)
    // Screen Y down  (+) -> Game Z (+) (South / Backward)
    const dx = normX * step;
    const dz = normY * step;
    vectorRef.current = { dx, dz };

    if (Math.hypot(normX, normY) < 0.1) {
      setStatusText('Idle');
    } else {
      let deg = Math.round((Math.atan2(normY, normX) * 180) / Math.PI) + 90;
      if (deg < 0) deg += 360;
      let cardinal = 'N';
      if (deg >= 22.5 && deg < 67.5) cardinal = 'NE';
      else if (deg >= 67.5 && deg < 112.5) cardinal = 'E';
      else if (deg >= 112.5 && deg < 157.5) cardinal = 'SE';
      else if (deg >= 157.5 && deg < 202.5) cardinal = 'S';
      else if (deg >= 202.5 && deg < 247.5) cardinal = 'SW';
      else if (deg >= 247.5 && deg < 292.5) cardinal = 'W';
      else if (deg >= 292.5 && deg < 337.5) cardinal = 'NW';

      setStatusText(`${cardinal} ${deg}°`);
    }
  }, []);

  // Continuous loop while dragging
  const startMovingLoop = useCallback(() => {
    if (moveTimerRef.current !== null) return;
    moveTimerRef.current = window.setInterval(() => {
      const { dx, dz } = vectorRef.current;
      if (Math.hypot(dx, dz) > 0.05) {
        onMoveVector?.(dx, dz);
        void multiRunCommand(target, 'main.movement.move', { dx, dz });
      }
    }, 100);
  }, [onMoveVector, target]);

  const stopMovingLoop = useCallback(() => {
    if (moveTimerRef.current !== null) {
      clearInterval(moveTimerRef.current);
      moveTimerRef.current = null;
    }
    vectorRef.current = { dx: 0, dz: 0 };
    onStop?.();
    void multiRunCommand(target, 'main.movement.stop', {});
  }, [onStop, target]);

  // Clean up on unmount
  useEffect(() => {
    return () => {
      if (moveTimerRef.current !== null) {
        clearInterval(moveTimerRef.current);
      }
    };
  }, []);

  // Pointer event handlers with setPointerCapture
  const handlePointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    if (activePointerId.current !== null) return;
    const base = baseRef.current;
    if (!base) return;

    activePointerId.current = e.pointerId;
    base.setPointerCapture(e.pointerId);
    setIsDragging(true);

    const rect = base.getBoundingClientRect();
    const centerX = rect.left + rect.width / 2;
    const centerY = rect.top + rect.height / 2;

    const rawX = e.clientX - centerX;
    const rawY = e.clientY - centerY;
    const dist = Math.hypot(rawX, rawY);
    const clampedDist = Math.min(dist, MAX_RADIUS);
    const rad = Math.atan2(rawY, rawX);

    const knobX = Math.cos(rad) * clampedDist;
    const knobY = Math.sin(rad) * clampedDist;

    setPosition({ x: knobX, y: knobY });
    updateDirection(knobX / MAX_RADIUS, knobY / MAX_RADIUS, stepSize);
    const dx = (knobX / MAX_RADIUS) * stepSize;
    const dz = (knobY / MAX_RADIUS) * stepSize;
    onMoveVector?.(dx, dz);
    void multiRunCommand(target, 'main.movement.move', { dx, dz });
    startMovingLoop();
  };

  const handlePointerMove = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!isDragging || activePointerId.current !== e.pointerId) return;
    const base = baseRef.current;
    if (!base) return;

    const rect = base.getBoundingClientRect();
    const centerX = rect.left + rect.width / 2;
    const centerY = rect.top + rect.height / 2;

    const rawX = e.clientX - centerX;
    const rawY = e.clientY - centerY;
    const dist = Math.hypot(rawX, rawY);
    const clampedDist = Math.min(dist, MAX_RADIUS);
    const rad = Math.atan2(rawY, rawX);

    const knobX = Math.cos(rad) * clampedDist;
    const knobY = Math.sin(rad) * clampedDist;

    setPosition({ x: knobX, y: knobY });
    updateDirection(knobX / MAX_RADIUS, knobY / MAX_RADIUS, stepSize);
  };

  const handlePointerUp = (e: React.PointerEvent<HTMLDivElement>) => {
    if (activePointerId.current !== e.pointerId) return;
    const base = baseRef.current;
    if (base && base.hasPointerCapture(e.pointerId)) {
      base.releasePointerCapture(e.pointerId);
    }
    activePointerId.current = null;
    setIsDragging(false);
    setPosition({ x: 0, y: 0 });
    setStatusText('Idle');
    stopMovingLoop();
  };

  // Keyboard controls (WASD / Arrows)
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    let nx = 0;
    let ny = 0;
    switch (e.key.toLowerCase()) {
      case 'w':
      case 'arrowup':
        ny = -1;
        break;
      case 's':
      case 'arrowdown':
        ny = 1;
        break;
      case 'a':
      case 'arrowleft':
        nx = -1;
        break;
      case 'd':
      case 'arrowright':
        nx = 1;
        break;
      default:
        return;
    }
    e.preventDefault();
    const knobX = nx * MAX_RADIUS * 0.8;
    const knobY = ny * MAX_RADIUS * 0.8;
    setPosition({ x: knobX, y: knobY });
    updateDirection(nx, ny, stepSize);
    const dx = nx * stepSize;
    const dz = ny * stepSize;
    onMoveVector?.(dx, dz);
    void multiRunCommand(target, 'main.movement.move', { dx, dz });
    startMovingLoop();
  };

  const handleKeyUp = () => {
    setPosition({ x: 0, y: 0 });
    setStatusText('Idle');
    stopMovingLoop();
  };

  return (
    <div className="pn" style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: 'var(--head)',
          padding: '7px 10px',
        }}
      >
        <h5 style={{ margin: 0, padding: 0, background: 'none' }}>
          <Icon name="open_with" />
          Movement
        </h5>
        <span
          style={{
            fontSize: '10px',
            fontFamily: 'var(--mono)',
            fontWeight: 700,
            color: isDragging ? 'var(--g)' : 'var(--m)',
            background: 'rgba(0, 0, 0, 0.4)',
            padding: '2px 6px',
            borderRadius: '3px',
            border: `1px solid ${isDragging ? 'rgba(85, 255, 153, 0.3)' : 'var(--ln)'}`,
            userSelect: 'none',
          }}
        >
          {statusText}
        </span>
      </div>

      <div
        className="pb"
        tabIndex={0}
        onKeyDown={handleKeyDown}
        onKeyUp={handleKeyUp}
        style={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          gap: '8px',
          outline: 'none',
        }}
      >
        {/* Virtual Joystick Base */}
        <div
          ref={baseRef}
          onPointerDown={handlePointerDown}
          onPointerMove={handlePointerMove}
          onPointerUp={handlePointerUp}
          onPointerCancel={handlePointerUp}
          style={{
            width: `${BASE_SIZE}px`,
            height: `${BASE_SIZE}px`,
            borderRadius: '50%',
            background: 'radial-gradient(circle at 50% 50%, #1e1115 0%, #140a0e 65%, #0d0608 100%)',
            border: isDragging ? '2px solid #ff4d6d' : '2px solid #3d1a22',
            boxShadow: isDragging
              ? '0 0 14px rgba(255, 77, 109, 0.3), inset 0 2px 10px rgba(0, 0, 0, 0.85)'
              : 'inset 0 2px 8px rgba(0, 0, 0, 0.8), 0 2px 6px rgba(0, 0, 0, 0.4)',
            position: 'relative',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            touchAction: 'none',
            userSelect: 'none',
            cursor: isDragging ? 'grabbing' : 'grab',
            transition: 'border-color 0.2s, box-shadow 0.2s',
          }}
        >
          {/* Crosshair guide lines */}
          <div
            style={{
              position: 'absolute',
              top: '12px',
              bottom: '12px',
              left: '50%',
              width: '1px',
              background: 'repeating-linear-gradient(to bottom, #2d1419, #2d1419 3px, transparent 3px, transparent 6px)',
              pointerEvents: 'none',
            }}
          />
          <div
            style={{
              position: 'absolute',
              left: '12px',
              right: '12px',
              top: '50%',
              height: '1px',
              background: 'repeating-linear-gradient(to right, #2d1419, #2d1419 3px, transparent 3px, transparent 6px)',
              pointerEvents: 'none',
            }}
          />

          {/* Cardinal direction indicators */}
          <span
            style={{
              position: 'absolute',
              top: '5px',
              fontSize: '9px',
              fontWeight: 800,
              color: isDragging && position.y < -10 ? 'var(--g)' : '#5a2a33',
              pointerEvents: 'none',
            }}
          >
            N
          </span>
          <span
            style={{
              position: 'absolute',
              bottom: '5px',
              fontSize: '9px',
              fontWeight: 800,
              color: isDragging && position.y > 10 ? 'var(--g)' : '#5a2a33',
              pointerEvents: 'none',
            }}
          >
            S
          </span>
          <span
            style={{
              position: 'absolute',
              left: '6px',
              fontSize: '9px',
              fontWeight: 800,
              color: isDragging && position.x < -10 ? 'var(--g)' : '#5a2a33',
              pointerEvents: 'none',
            }}
          >
            W
          </span>
          <span
            style={{
              position: 'absolute',
              right: '6px',
              fontSize: '9px',
              fontWeight: 800,
              color: isDragging && position.x > 10 ? 'var(--g)' : '#5a2a33',
              pointerEvents: 'none',
            }}
          >
            E
          </span>

          {/* Joystick Knob / Puck */}
          <div
            style={{
              width: '46px',
              height: '46px',
              borderRadius: '50%',
              background: isDragging
                ? 'radial-gradient(circle at 35% 35%, #6e1c28 0%, #360c14 70%, #1f060b 100%)'
                : 'radial-gradient(circle at 35% 35%, #4a151e 0%, #29090f 70%, #170407 100%)',
              border: isDragging ? '2px solid #ff4d6d' : '2px solid #6b1f2b',
              boxShadow: isDragging
                ? '0 4px 12px rgba(255, 77, 109, 0.4), inset 0 1px 3px rgba(255, 255, 255, 0.3)'
                : '0 3px 8px rgba(0, 0, 0, 0.65), inset 0 1px 2px rgba(255, 255, 255, 0.15)',
              transform: `translate3d(${position.x}px, ${position.y}px, 0)`,
              transition: isDragging ? 'none' : 'transform 0.18s cubic-bezier(0.175, 0.885, 0.32, 1.275)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              pointerEvents: 'none',
            }}
          >
            {/* Center grip pip */}
            <div
              style={{
                width: '12px',
                height: '12px',
                borderRadius: '50%',
                background: isDragging ? '#ff4d6d' : '#8a2332',
                boxShadow: isDragging ? '0 0 6px #ff4d6d' : 'none',
                transition: 'background 0.2s',
              }}
            />
          </div>
        </div>

        {/* Step Speed and Action Bar */}
        <div style={{ display: 'flex', gap: '6px', alignItems: 'center', width: '100%', marginTop: '2px' }}>
          {/* multi:ui-only unwired noop */}
          <button
            className="b btn-icon"
            style={{ width: '28px', padding: 0, display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}
            title="Recall / Return"
            onClick={onDoor}
          >
            <Icon name="door_open" />
          </button>

          <button
            className="b"
            style={{ width: '28px', padding: 0, fontWeight: 700 }}
            title="Decrease Step"
            onClick={() => updateStep(stepSize - 0.2)}
          >
            -
          </button>

          <div
            style={{
              flex: 1,
              display: 'flex',
              alignItems: 'center',
              background: '#0c0304',
              border: '1px solid var(--ln)',
              borderRadius: '3px',
              padding: '0 6px',
              height: '24px',
            }}
          >
            <span style={{ fontSize: '10px', color: 'var(--m)', marginRight: '4px' }}>Step:</span>
            <input
              className="inp"
              type="number"
              min="0.2"
              max="20"
              step="0.2"
              value={stepSize}
              onChange={(e) => {
                const val = parseFloat(e.target.value);
                if (!isNaN(val) && val > 0) updateStep(val);
              }}
              style={{
                border: 'none',
                background: 'transparent',
                textAlign: 'center',
                padding: 0,
                height: '100%',
                fontSize: '11px',
                fontFamily: 'var(--mono)',
                color: 'var(--w)',
              }}
            />
          </div>

          <button
            className="b"
            style={{ width: '28px', padding: 0, fontWeight: 700 }}
            title="Increase Step"
            onClick={() => updateStep(stepSize + 0.2)}
          >
            +
          </button>

          <button
            className="b btn-icon"
            style={{ width: '28px', padding: 0, display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}
            title="Reset to 1.2"
            onClick={() => updateStep(1.2)}
          >
            <Icon name="settings" />
          </button>
        </div>
      </div>
    </div>
  );
};
