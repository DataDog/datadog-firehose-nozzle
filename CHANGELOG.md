# Changelog

## 2.5.0 / 2022-11-10

* [Added] Add option to disable application metadata prefix. See [#93](https://github.com/DataDog/datadog-firehose-nozzle/pull/93).
* [Fixed] Fix SSL errors when using the Cluster Agent API. See [#96](https://github.com/DataDog/datadog-firehose-nozzle/pull/96).

## 2.4.1 / 2022-04-27

* [Fixed] Fix sidecars tagging. See [#90](https://github.com/DataDog/datadog-firehose-nozzle/pull/90).

## 2.4.0 / 2022-04-13

* [Added] Add sidecars tags to app metrics. See [#88](https://github.com/DataDog/datadog-firehose-nozzle/pull/88).
* [Added] Automatically add tags to orgs metrics based on metadata. See [#87](https://github.com/DataDog/datadog-firehose-nozzle/pull/87).
* [Added] Add support for the DCA API. See [#86](https://github.com/DataDog/datadog-firehose-nozzle/pull/86).
* [Added] Improve telemetry of the nozzle. See [#85](https://github.com/DataDog/datadog-firehose-nozzle/pull/85).
* [Added] Improve metric submission logic to split metric packets. See [#84](https://github.com/DataDog/datadog-firehose-nozzle/pull/84).

## 2.3.0 / 2021-08-05

* [Added] Automatically add tags to application metrics based on metadata. See [#81](https://github.com/DataDog/datadog-firehose-nozzle/pull/81).
* [Added] Upgrade go version to 1.16 and move to go modules. See [#82](https://github.com/DataDog/datadog-firehose-nozzle/pull/82).

## 2.2.0 / 2020-07-23

* [Added] Add labels and annotations as tags. See [#79](https://github.com/DataDog/datadog-firehose-nozzle/pull/79).
