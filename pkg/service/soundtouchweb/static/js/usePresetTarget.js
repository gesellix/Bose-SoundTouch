import { useLayoutEffect, useRef } from 'preact/hooks';

export function usePresetTarget(deviceId, physicalId, epoch, onStatusReadback) {
    const owner = useRef(null);
    const key = JSON.stringify([deviceId, physicalId, epoch]);
    if (owner.current?.key !== key) owner.current = { key };
    const captured = owner.current;
    useLayoutEffect(() => () => { owner.current = null; }, []);
    return { deviceId, physicalId, epoch, onStatusReadback, isCurrent: () => owner.current === captured };
}
