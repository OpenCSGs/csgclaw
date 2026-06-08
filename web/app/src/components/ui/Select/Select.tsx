import { Select as RadixSelect } from "radix-ui";
import { Check, ChevronDown, ChevronUp } from "lucide-react";
import { forwardRef } from "react";
import type { ComponentPropsWithoutRef, ComponentRef, ReactNode } from "react";
import { classNames } from "@/shared/lib/classNames";

export type SelectSize = "sm" | "md";

export type SelectOption = {
  disabled?: boolean;
  label: ReactNode;
  textValue?: string;
  value: string;
};

const triggerSizeClassNames: Record<SelectSize, string> = {
  sm: "csg-select-trigger-sm",
  md: "csg-select-trigger-md",
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
    return (
      <RadixSelect.Trigger
        ref={ref}
        className={classNames(
          "csg-select-trigger inline-flex w-full items-center justify-between gap-2",
          triggerSizeClassNames[size],
          className,
        )}
        {...props}
      >
        <span className="csg-select-value min-w-0 flex-1 truncate">
          {children ?? <RadixSelect.Value placeholder={placeholder} />}
        </span>
        <RadixSelect.Icon asChild>
          <ChevronDown className="csg-select-chevron shrink-0" aria-hidden="true" size={16} strokeWidth={2} />
        </RadixSelect.Icon>
      </RadixSelect.Trigger>
    );
  },
);

export type SelectContentProps = ComponentPropsWithoutRef<typeof RadixSelect.Content> & {
  portalContainer?: ComponentPropsWithoutRef<typeof RadixSelect.Portal>["container"];
};

export const SelectContent = forwardRef<ComponentRef<typeof RadixSelect.Content>, SelectContentProps>(
  function SelectContent({ children, className, portalContainer, position = "popper", sideOffset = 6, ...props }, ref) {
    return (
      <RadixSelect.Portal container={portalContainer}>
        <RadixSelect.Content
          ref={ref}
          className={classNames("csg-select-content overflow-hidden", className)}
          position={position}
          sideOffset={sideOffset}
          {...props}
        >
          <RadixSelect.ScrollUpButton className="csg-select-scroll-button flex items-center justify-center">
            <ChevronUp aria-hidden="true" size={16} strokeWidth={2} />
          </RadixSelect.ScrollUpButton>
          <RadixSelect.Viewport className="csg-select-viewport p-1">{children}</RadixSelect.Viewport>
          <RadixSelect.ScrollDownButton className="csg-select-scroll-button flex items-center justify-center">
            <ChevronDown aria-hidden="true" size={16} strokeWidth={2} />
          </RadixSelect.ScrollDownButton>
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
  return <RadixSelect.Label ref={ref} className={classNames("csg-select-label px-2 py-1.5", className)} {...props} />;
});

export type SelectItemProps = ComponentPropsWithoutRef<typeof RadixSelect.Item>;

export const SelectItem = forwardRef<ComponentRef<typeof RadixSelect.Item>, SelectItemProps>(function SelectItem(
  { children, className, ...props },
  ref,
) {
  return (
    <RadixSelect.Item
      ref={ref}
      className={classNames("csg-select-item relative flex w-full items-center rounded-md py-2 pr-3 pl-8", className)}
      {...props}
    >
      <span className="csg-select-item-indicator absolute left-2 inline-flex items-center justify-center">
        <RadixSelect.ItemIndicator>
          <Check aria-hidden="true" size={16} strokeWidth={2} />
        </RadixSelect.ItemIndicator>
      </span>
      <RadixSelect.ItemText>{children}</RadixSelect.ItemText>
    </RadixSelect.Item>
  );
});

export type SelectSeparatorProps = ComponentPropsWithoutRef<typeof RadixSelect.Separator>;

export const SelectSeparator = forwardRef<ComponentRef<typeof RadixSelect.Separator>, SelectSeparatorProps>(
  function SelectSeparator({ className, ...props }, ref) {
    return (
      <RadixSelect.Separator
        ref={ref}
        className={classNames("csg-select-separator -mx-1 my-1", className)}
        {...props}
      />
    );
  },
);

export type SelectProps = Omit<SelectRootProps, "children"> & {
  children?: ReactNode;
  contentClassName?: string;
  contentProps?: Omit<SelectContentProps, "children" | "className">;
  options?: readonly SelectOption[];
  placeholder?: ReactNode;
  size?: SelectSize;
  triggerClassName?: string;
  triggerProps?: Omit<SelectTriggerProps, "children" | "className" | "placeholder" | "size">;
};

export function Select({
  children,
  contentClassName,
  contentProps,
  defaultValue,
  onValueChange,
  options,
  placeholder,
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
      <SelectTrigger className={triggerClassName} placeholder={placeholder} size={size} {...triggerProps} />
      <SelectContent className={contentClassName} {...contentProps}>
        {children ??
          options?.map((option) => (
            <SelectItem
              key={option.value}
              value={toRadixValue(option.value, hasEmptyOption) ?? option.value}
              disabled={option.disabled}
              textValue={option.textValue}
            >
              {option.label}
            </SelectItem>
          ))}
      </SelectContent>
    </SelectRoot>
  );
}
