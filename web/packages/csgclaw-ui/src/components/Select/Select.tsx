/*! Derived from CSGClaw. Modified for standalone csgclaw-ui distribution. */
import styles from "./Select.module.css";
import { Select as RadixSelect } from "radix-ui";
import { Check, ChevronDown } from "lucide-react";
import { Fragment, forwardRef } from "react";
import type { ComponentPropsWithoutRef, ComponentRef, ReactNode } from "react";
import { useFieldControlProps } from "../FormControls/fieldContext.js";
import { classNames, legacyClassName } from "../../lib/classNames.js";

export type SelectSize = "sm" | "md";

export type SelectOption = {
  description?: ReactNode;
  disabled?: boolean;
  label: ReactNode;
  separatorAfter?: boolean;
  textValue?: string;
  value: string;
};

const triggerSizeClassNames: Record<SelectSize, string> = {
  sm: styles["csg-select-trigger-sm"],
  md: styles["csg-select-trigger-md"],
};

const SELECT_EMPTY_ITEM_VALUE = "__csg-select-empty-value__";

function toRadixValue(value: string | undefined, hasEmptyOption: boolean) {
  return hasEmptyOption && value === "" ? SELECT_EMPTY_ITEM_VALUE : value;
}

function fromRadixValue(value: string) {
  return value === SELECT_EMPTY_ITEM_VALUE ? "" : value;
}

export type SelectRootProps = ComponentPropsWithoutRef<typeof RadixSelect.Root>;

export function SelectRoot(props: SelectRootProps) {
  return <RadixSelect.Root {...props} />;
}

export type SelectGroupProps = ComponentPropsWithoutRef<typeof RadixSelect.Group>;

export const SelectGroup = forwardRef<ComponentRef<typeof RadixSelect.Group>, SelectGroupProps>(
  function SelectGroup(props, ref) {
    return <RadixSelect.Group ref={ref} {...props} />;
  },
);

export type SelectValueProps = ComponentPropsWithoutRef<typeof RadixSelect.Value>;

export const SelectValue = forwardRef<ComponentRef<typeof RadixSelect.Value>, SelectValueProps>(
  function SelectValue(props, ref) {
    return <RadixSelect.Value ref={ref} {...props} />;
  },
);

export type SelectTriggerProps = ComponentPropsWithoutRef<typeof RadixSelect.Trigger> & {
  placeholder?: SelectValueProps["placeholder"];
  size?: SelectSize;
};

export const SelectTrigger = forwardRef<ComponentRef<typeof RadixSelect.Trigger>, SelectTriggerProps>(
  function SelectTrigger({ children, className, placeholder, size = "md", ...props }, ref) {
    const controlProps = useFieldControlProps(props);
    return (
      <RadixSelect.Trigger
        ref={ref}
        data-csg-control="select"
        className={classNames(
          classNames(styles["csg-select-trigger"], legacyClassName("csg-select-trigger")),
          triggerSizeClassNames[size],
          className,
        )}
        {...controlProps}
      >
        <span className={classNames(styles["csg-select-value"], legacyClassName("csg-select-value"))}>
          {children ?? <RadixSelect.Value placeholder={placeholder} />}
        </span>
        <RadixSelect.Icon asChild>
          <ChevronDown
            className={classNames(styles["csg-select-chevron"], legacyClassName("csg-select-chevron"))}
            aria-hidden="true"
            size={16}
            strokeWidth={2}
          />
        </RadixSelect.Icon>
      </RadixSelect.Trigger>
    );
  },
);

export type SelectContentProps = ComponentPropsWithoutRef<typeof RadixSelect.Content> & {
  header?: ReactNode;
  portalContainer?: ComponentPropsWithoutRef<typeof RadixSelect.Portal>["container"];
};

export const SelectContent = forwardRef<ComponentRef<typeof RadixSelect.Content>, SelectContentProps>(
  function SelectContent(
    { children, className, header, portalContainer, position = "popper", sideOffset = 6, ...props },
    ref,
  ) {
    return (
      <RadixSelect.Portal container={portalContainer}>
        <RadixSelect.Content
          ref={ref}
          className={classNames(styles["csg-select-content"], className)}
          position={position}
          sideOffset={sideOffset}
          {...props}
        >
          {header}
          <RadixSelect.Viewport className={styles["csg-select-viewport"]}>{children}</RadixSelect.Viewport>
        </RadixSelect.Content>
      </RadixSelect.Portal>
    );
  },
);

export type SelectLabelProps = ComponentPropsWithoutRef<typeof RadixSelect.Label>;

export const SelectLabel = forwardRef<ComponentRef<typeof RadixSelect.Label>, SelectLabelProps>(function SelectLabel(
  { className, ...props },
  ref,
) {
  return <RadixSelect.Label ref={ref} className={classNames(styles["csg-select-label"], className)} {...props} />;
});

export type SelectItemProps = ComponentPropsWithoutRef<typeof RadixSelect.Item> & {
  description?: ReactNode;
};

export const SelectItem = forwardRef<ComponentRef<typeof RadixSelect.Item>, SelectItemProps>(function SelectItem(
  { children, className, description, ...props },
  ref,
) {
  return (
    <RadixSelect.Item
      ref={ref}
      className={classNames(
        styles["csg-select-item"],
        description ? styles["csg-select-item-with-description"] : "",
        className,
      )}
      {...props}
    >
      <span className={styles["csg-select-item-indicator"]}>
        <RadixSelect.ItemIndicator>
          <Check aria-hidden="true" size={16} strokeWidth={2} />
        </RadixSelect.ItemIndicator>
      </span>
      <span className={styles["csg-select-item-copy"]}>
        <RadixSelect.ItemText>{children}</RadixSelect.ItemText>
        {description ? <span className={styles["csg-select-item-description"]}>{description}</span> : null}
      </span>
    </RadixSelect.Item>
  );
});

export type SelectSeparatorProps = ComponentPropsWithoutRef<typeof RadixSelect.Separator>;

export const SelectSeparator = forwardRef<ComponentRef<typeof RadixSelect.Separator>, SelectSeparatorProps>(
  function SelectSeparator({ className, ...props }, ref) {
    return (
      <RadixSelect.Separator ref={ref} className={classNames(styles["csg-select-separator"], className)} {...props} />
    );
  },
);

export type SelectProps = Omit<SelectRootProps, "children"> & {
  children?: ReactNode;
  contentClassName?: string;
  contentProps?: Omit<SelectContentProps, "children" | "className">;
  emptyLabel?: ReactNode;
  options?: readonly SelectOption[];
  placeholder?: ReactNode;
  selectedLabel?: ReactNode;
  size?: SelectSize;
  triggerClassName?: string;
  triggerProps?: Omit<SelectTriggerProps, "children" | "className" | "placeholder" | "size">;
};

export function Select({
  children,
  contentClassName,
  contentProps,
  defaultValue,
  emptyLabel = "No options",
  onValueChange,
  options,
  placeholder,
  selectedLabel,
  size = "md",
  triggerClassName,
  triggerProps,
  value,
  ...props
}: SelectProps) {
  const hasEmptyOption = Boolean(options?.some((option) => option.value === ""));
  return (
    <SelectRoot
      value={toRadixValue(value, hasEmptyOption)}
      defaultValue={toRadixValue(defaultValue, hasEmptyOption)}
      onValueChange={onValueChange ? (nextValue) => onValueChange(fromRadixValue(nextValue)) : undefined}
      {...props}
    >
      <SelectTrigger className={triggerClassName} placeholder={placeholder} size={size} {...triggerProps}>
        {selectedLabel ?? (value && options && !options.some((option) => option.value === value) ? value : undefined)}
      </SelectTrigger>
      <SelectContent className={contentClassName} {...contentProps}>
        {children ??
          (options?.length ? (
            options.map((option, index) => (
              <Fragment key={option.value}>
                <SelectItem
                  value={toRadixValue(option.value, hasEmptyOption) ?? option.value}
                  disabled={option.disabled}
                  description={option.description}
                  textValue={option.textValue}
                >
                  {option.label}
                </SelectItem>
                {option.separatorAfter && index < options.length - 1 ? <SelectSeparator /> : null}
              </Fragment>
            ))
          ) : (
            <div className={styles["csg-select-empty"]}>{emptyLabel}</div>
          ))}
      </SelectContent>
    </SelectRoot>
  );
}
