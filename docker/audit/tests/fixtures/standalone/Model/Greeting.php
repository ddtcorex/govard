<?php
/**
 * Copyright (c) Govard contributors.
 * Distributed under the terms of the repository LICENSE file.
 */

declare(strict_types=1);

namespace Govard\SampleModule\Model;

/**
 * Minimal deterministic model used to exercise the lint toolchain.
 */
class Greeting
{
    /**
     * @var string
     */
    private $subject;

    /**
     * @param string $subject
     */
    public function __construct(string $subject)
    {
        $this->subject = $subject;
    }

    /**
     * Build the greeting message.
     *
     * @return string
     */
    public function message(): string
    {
        return 'Hello, ' . $this->subject . '.';
    }
}
